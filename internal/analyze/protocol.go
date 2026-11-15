package analyze

// Passive, bounded protocol decoding. Original bytes are always retained by the
// caller. A malformed/oversized protocol disables decoding for that direction;
// it never stalls capture or silently replaces the evidence.
import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"gomate/internal/model"
)

const MessageLimit = 256 << 10
const HeaderLimit = 32 << 10
const h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

type protocolDirection struct {
	buf       []byte
	mode      string
	ws        []byte
	wsOpcode  byte
	hpack     *hpack.Decoder
	header    []byte
	headerID  uint32
	headerEnd bool
}
type rpcStream struct {
	contentType     [2]string
	contentEncoding [2]string
	path            string
	method          string
	grpc            [2]bool
	encoding        [2]string
	buf             [2][]byte
	ended           [2]bool
}
type Decoder struct {
	Requests      int64 // Complete HTTP requests or HTTP/2 request header blocks.
	dirs          [2]protocolDirection
	streams       map[uint32]*rpcStream
	methods       []string
	UserAgentHash *string
}

func (d *Decoder) Size() int {
	n := len(d.methods) * 16
	for i := range d.dirs {
		x := &d.dirs[i]
		n += cap(x.buf) + cap(x.ws) + cap(x.header)
		if x.hpack != nil {
			n += 8192
		}
	}
	for _, s := range d.streams {
		n += cap(s.buf[0]) + cap(s.buf[1]) + len(s.path) + len(s.method) + 256
	}
	return n
}
func (d *Decoder) Reset() { *d = Decoder{} }
func (d *Decoder) Feed(p model.Packet, svc model.Service, eof bool) []model.Packet {
	idx := 0
	if p.Incoming {
		idx = 1
	}
	x := &d.dirs[idx]
	if x.mode == "failed" {
		return nil
	}
	fail := func(reason string) []model.Packet {
		x.buf = nil
		x.ws = nil
		x.header = nil
		x.hpack = nil
		x.mode = "failed"
		return []model.Packet{{Timestamp: p.Timestamp, Incoming: p.Incoming, Content: []byte(reason), Kind: "decode-error", Incomplete: true, Matches: []model.Match{}}}
	}
	if len(x.buf)+len(p.Content) > MessageLimit {
		return fail("protocol buffer limit exceeded; raw bytes retained")
	}
	x.buf = append(x.buf, p.Content...)
	var out []model.Packet
	emitted := 0
	emit := func(kind string, b []byte, meta map[string]string, gzip, ws, body bool) {
		emitted += len(b)
		out = append(out, model.Packet{Timestamp: p.Timestamp, Incoming: p.Incoming, Content: b, Kind: kind, Metadata: meta, Ungzipped: gzip, WebSocketParsed: ws, HasHTTPBody: body, Matches: []model.Match{}})
	}
	emitH2Body := func(s *rpcStream, id uint32) error {
		data := s.buf[idx]
		s.buf[idx] = nil
		gz := strings.EqualFold(s.contentEncoding[idx], "gzip")
		var err error
		if gz {
			data, err = gunzip(data)
			if err != nil {
				return err
			}
		}
		if svc.URLDecode && p.Incoming && strings.HasPrefix(s.contentType[idx], "application/x-www-form-urlencoded") {
			if value, e := url.QueryUnescape(string(data)); e == nil {
				data = []byte(value)
			}
		}
		emit("http2-body", data, map[string]string{"streamId": fmt.Sprint(id), "path": s.path, "contentType": s.contentType[idx]}, gz, false, true)
		return nil
	}

	for len(x.buf) > 0 {
		if emitted >= MessageLimit {
			return append(out, fail("decoded output budget exceeded")...)
		}
		if x.mode == "" {
			if len(x.buf) < 24 && !eof {
				break
			}
			if bytes.HasPrefix(x.buf, []byte(h2Preface)) {
				x.mode = "h2"
				x.buf = x.buf[len(h2Preface):]
			} else if len(x.buf) >= 9 && x.buf[3] == 4 && binary.BigEndian.Uint32(x.buf[5:9]) == 0 {
				x.mode = "h2"
			} else if bytes.HasPrefix(x.buf, []byte("HTTP/1.")) || isRequest(x.buf) {
				x.mode = "http"
			} else {
				return append(out, fail("unrecognized protocol or capture began mid-message; raw bytes retained")...)
			}
		}
		if x.mode == "http" {
			end := bytes.Index(x.buf, []byte("\r\n\r\n"))
			if end < 0 {
				if len(x.buf) > HeaderLimit || eof {
					return append(out, fail("incomplete HTTP headers")...)
				}
				break
			}
			if end > HeaderLimit {
				return append(out, fail("HTTP header limit exceeded")...)
			}
			source := bytes.NewReader(x.buf)
			br := bufio.NewReader(source)
			var body io.ReadCloser
			var headers http.Header
			var start string
			var length int64
			var chunked bool
			var method string
			var requestPath string
			var upgrade bool
			if p.Incoming {
				req, err := http.ReadRequest(br)
				if err != nil {
					return append(out, fail("invalid HTTP request: "+err.Error())...)
				}
				headers = req.Header
				body = req.Body
				length = req.ContentLength
				chunked = len(req.TransferEncoding) > 0
				method = req.Method
				requestPath = req.RequestURI
				target := req.RequestURI
				if svc.URLDecode {
					if v, e := url.PathUnescape(target); e == nil {
						target = v
					}
				}
				start = req.Method + " " + target + " " + req.Proto + "\r\nHost: " + req.Host + "\r\n"
				upgrade = strings.EqualFold(headers.Get("Upgrade"), "websocket")
			} else {
				request := &http.Request{Method: "GET"}
				if len(d.methods) > 0 {
					request.Method = d.methods[0]
				}
				res, err := http.ReadResponse(br, request)
				if err != nil {
					return append(out, fail("invalid HTTP response: "+err.Error())...)
				}
				headers = res.Header
				body = res.Body
				length = res.ContentLength
				chunked = len(res.TransferEncoding) > 0
				start = res.Proto + " " + res.Status + "\r\n"
				upgrade = res.StatusCode == 101 && strings.EqualFold(headers.Get("Upgrade"), "websocket")
				if res.StatusCode < 200 && res.StatusCode != 101 {
					method = "interim"
				}
				if length < 0 && !chunked && !eof && !upgrade {
					break
				}
			}
			if length > MessageLimit {
				return append(out, fail("HTTP body limit exceeded")...)
			}
			data, err := io.ReadAll(io.LimitReader(body, MessageLimit+1))
			if err != nil {
				if !eof && (err == io.ErrUnexpectedEOF || err == io.EOF) {
					break
				}
				return append(out, fail("invalid or truncated HTTP body")...)
			}
			if len(data) > MessageLimit {
				return append(out, fail("HTTP body limit exceeded")...)
			}
			used := len(x.buf) - source.Len() - br.Buffered()
			if used <= 0 {
				return append(out, fail("HTTP parser made no progress")...)
			}
			if p.Incoming {
				if len(d.methods) >= 1024 {
					return append(out, fail("HTTP pipeline limit exceeded")...)
				}
				d.methods = append(d.methods, method)
				if ua := headers.Get("User-Agent"); ua != "" {
					hash := sha256.Sum256([]byte(ua))
					v := hex.EncodeToString(hash[:8])
					d.UserAgentHash = &v
				}
			} else if method != "interim" && len(d.methods) > 0 {
				d.methods = d.methods[1:]
			}
			gz := strings.EqualFold(headers.Get("Content-Encoding"), "gzip")
			if gz {
				data, err = gunzip(data)
				if err != nil {
					return append(out, fail(err.Error())...)
				}
			}
			if svc.URLDecode && p.Incoming && strings.HasPrefix(headers.Get("Content-Type"), "application/x-www-form-urlencoded") {
				if v, e := url.QueryUnescape(string(data)); e == nil {
					data = []byte(v)
				}
			}
			var b bytes.Buffer
			b.WriteString(start)
			headers.Write(&b)
			b.WriteString("\r\n")
			b.Write(data)
			meta := map[string]string{"contentType": headers.Get("Content-Type")}
			if p.Incoming {
				meta["path"] = requestPath
				meta["method"] = method
			}
			emit("http", b.Bytes(), meta, gz, false, len(data) > 0)
			if p.Incoming {
				d.Requests++
			}
			x.buf = append([]byte(nil), x.buf[used:]...)
			if upgrade && svc.WebSockets {
				x.mode = "ws"
			}
		} else if x.mode == "ws" {
			if len(x.buf) < 2 {
				break
			}
			b := x.buf
			fin := b[0]&128 != 0
			op := b[0] & 15
			masked := b[1]&128 != 0
			if b[0]&112 != 0 {
				return append(out, fail("WebSocket extensions are not decoded")...)
			}
			n := uint64(b[1] & 127)
			off := 2
			if n == 126 {
				if len(b) < 4 {
					break
				}
				n = uint64(binary.BigEndian.Uint16(b[2:4]))
				off = 4
			} else if n == 127 {
				if len(b) < 10 {
					break
				}
				n = binary.BigEndian.Uint64(b[2:10])
				off = 10
			}
			if n > MessageLimit {
				return append(out, fail("WebSocket frame limit exceeded")...)
			}
			if masked {
				off += 4
			}
			if len(b) < off+int(n) {
				break
			}
			payload := append([]byte(nil), b[off:off+int(n)]...)
			if masked {
				for i := range payload {
					payload[i] ^= b[off-4+i%4]
				}
			}
			x.buf = append([]byte(nil), b[off+int(n):]...)
			if op >= 8 {
				if !fin || n > 125 || op > 10 {
					return append(out, fail("invalid WebSocket control frame")...)
				}
				emit("websocket-control", payload, map[string]string{"opcode": fmt.Sprint(op)}, false, true, false)
				continue
			}
			if op > 2 || op == 0 && x.wsOpcode == 0 || op != 0 && x.wsOpcode != 0 {
				return append(out, fail("invalid WebSocket continuation")...)
			}
			if op != 0 {
				x.wsOpcode = op
			}
			if len(x.ws)+len(payload) > MessageLimit {
				return append(out, fail("WebSocket message limit exceeded")...)
			}
			x.ws = append(x.ws, payload...)
			if fin {
				emit("websocket", x.ws, map[string]string{"opcode": fmt.Sprint(x.wsOpcode)}, false, true, false)
				x.ws = nil
				x.wsOpcode = 0
			}
		} else if x.mode == "h2" {
			if len(x.buf) < 9 {
				break
			}
			n := int(x.buf[0])<<16 | int(x.buf[1])<<8 | int(x.buf[2])
			if n > MessageLimit-9 {
				return append(out, fail("HTTP/2 frame limit exceeded")...)
			}
			if len(x.buf) < 9+n {
				break
			}
			fr := http2.NewFramer(nil, bytes.NewReader(x.buf[:9+n]))
			fr.AllowIllegalReads = true
			f, err := fr.ReadFrame()
			if err != nil {
				return append(out, fail("invalid HTTP/2 frame: "+err.Error())...)
			}
			if x.headerID != 0 {
				if cf, ok := f.(*http2.ContinuationFrame); !ok || cf.StreamID != x.headerID {
					return append(out, fail("missing HTTP/2 CONTINUATION")...)
				}
			}
			id := f.Header().StreamID
			get := func() *rpcStream {
				if d.streams == nil {
					d.streams = map[uint32]*rpcStream{}
				}
				s := d.streams[id]
				if s == nil && len(d.streams) < 128 {
					s = &rpcStream{}
					d.streams[id] = s
				}
				return s
			}
			var fragment []byte
			var headersEnd bool
			switch v := f.(type) {
			case *http2.HeadersFrame:
				x.headerID = id
				x.headerEnd = v.StreamEnded()
				fragment = v.HeaderBlockFragment()
				headersEnd = v.HeadersEnded()
			case *http2.ContinuationFrame:
				if x.headerID != id {
					return append(out, fail("unexpected HTTP/2 CONTINUATION")...)
				}
				fragment = v.HeaderBlockFragment()
				headersEnd = v.HeadersEnded()
			case *http2.DataFrame:
				s := get()
				if s == nil {
					return append(out, fail("HTTP/2 active stream limit exceeded")...)
				}
				if s.grpc[idx] {
					if len(s.buf[idx])+len(v.Data()) > MessageLimit {
						return append(out, fail("gRPC message buffer limit exceeded")...)
					}
					s.buf[idx] = append(s.buf[idx], v.Data()...)
					for len(s.buf[idx]) >= 5 {
						if emitted >= MessageLimit {
							return append(out, fail("decoded output budget exceeded")...)
						}
						b := s.buf[idx]
						sz := int(binary.BigEndian.Uint32(b[1:5]))
						if sz > MessageLimit-5 {
							return append(out, fail("gRPC message limit exceeded")...)
						}
						if len(b) < 5+sz {
							break
						}
						data := append([]byte(nil), b[5:5+sz]...)
						compressed := b[0] == 1
						if b[0] > 1 {
							return append(out, fail("invalid gRPC compression flag")...)
						}
						if compressed {
							if s.encoding[idx] != "gzip" {
								return append(out, fail("unsupported gRPC encoding")...)
							}
							data, err = gunzip(data)
							if err != nil {
								return append(out, fail(err.Error())...)
							}
						}
						emit("grpc", data, map[string]string{"streamId": fmt.Sprint(id), "path": s.path, "method": s.method, "encoding": s.encoding[idx]}, compressed, false, true)
						s.buf[idx] = append([]byte(nil), b[5+sz:]...)
					}
					if v.StreamEnded() && len(s.buf[idx]) != 0 {
						return append(out, fail("truncated gRPC message")...)
					}
				} else {
					if len(s.buf[idx])+len(v.Data()) > MessageLimit {
						return append(out, fail("HTTP/2 body limit exceeded")...)
					}
					s.buf[idx] = append(s.buf[idx], v.Data()...)
					if v.StreamEnded() {
						if err = emitH2Body(s, id); err != nil {
							return append(out, fail(err.Error())...)
						}
					}
				}
				s.ended[idx] = v.StreamEnded()
				if s.ended[0] && s.ended[1] {
					delete(d.streams, id)
				}
			case *http2.RSTStreamFrame:
				delete(d.streams, id)
			case *http2.GoAwayFrame:
				for k := range d.streams {
					if k > v.LastStreamID {
						delete(d.streams, k)
					}
				}
			}
			if fragment != nil {
				if len(x.header)+len(fragment) > HeaderLimit {
					return append(out, fail("HTTP/2 header block limit exceeded")...)
				}
				x.header = append(x.header, fragment...)
			}
			if headersEnd {
				s := get()
				if s == nil {
					return append(out, fail("HTTP/2 active stream limit exceeded")...)
				}
				if x.hpack == nil {
					x.hpack = hpack.NewDecoder(4096, nil)
					x.hpack.SetMaxStringLength(HeaderLimit)
					x.hpack.SetAllowedMaxDynamicTableSize(4096)
				}
				var hb bytes.Buffer
				tooLarge := false
				requestHeaders := false
				x.hpack.SetEmitFunc(func(h hpack.HeaderField) {
					if hb.Len()+len(h.Name)+len(h.Value)+4 > HeaderLimit {
						tooLarge = true
						return
					}
					if h.Name == ":path" && svc.URLDecode {
						if value, e := url.PathUnescape(h.Value); e == nil {
							h.Value = value
						}
					}
					hb.WriteString(h.Name + ": " + h.Value + "\r\n")
					switch h.Name {
					case ":method":
						s.method = h.Value
						requestHeaders = p.Incoming && h.Value != ""
					case ":path":
						s.path = h.Value
					case "content-encoding":
						s.contentEncoding[idx] = h.Value
					case "content-type":
						s.contentType[idx] = h.Value
						s.grpc[idx] = strings.HasPrefix(h.Value, "application/grpc")
					case "grpc-encoding":
						s.encoding[idx] = h.Value
					case "user-agent":
						hash := sha256.Sum256([]byte(h.Value))
						v := hex.EncodeToString(hash[:8])
						d.UserAgentHash = &v
					}
				})
				_, err = x.hpack.Write(x.header)
				if err == nil {
					err = x.hpack.Close()
				}
				x.hpack.SetEmitFunc(func(hpack.HeaderField) {})
				if err != nil || tooLarge {
					return append(out, fail("invalid or oversized HPACK headers")...)
				}
				if requestHeaders {
					d.Requests++
				}
				emit("http2-headers", hb.Bytes(), map[string]string{"streamId": fmt.Sprint(id), "path": s.path, "method": s.method}, false, false, false)
				if x.headerEnd {
					s.ended[idx] = true
					if len(s.buf[idx]) != 0 {
						if s.grpc[idx] {
							return append(out, fail("truncated gRPC message before trailers")...)
						}
						if err = emitH2Body(s, id); err != nil {
							return append(out, fail(err.Error())...)
						}
					}
				}
				if s.ended[0] && s.ended[1] {
					delete(d.streams, id)
				}
				x.header = nil
				x.headerID = 0
			}
			x.buf = append([]byte(nil), x.buf[9+n:]...)
		}
	}
	if eof {
		for _, s := range d.streams {
			if len(s.buf[idx]) > 0 {
				return append(out, fail("connection ended with incomplete HTTP/2 or gRPC body")...)
			}
		}
	}
	if eof && (len(x.buf) > 0 || len(x.ws) > 0 || x.headerID != 0) {
		return append(out, fail("connection ended with an incomplete protocol message")...)
	}
	return out
}
func isRequest(b []byte) bool {
	for _, m := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH ", "CONNECT ", "TRACE "} {
		if bytes.HasPrefix(b, []byte(m)) {
			return true
		}
	}
	return false
}
func gunzip(b []byte) ([]byte, error) {
	r, e := gzip.NewReader(bytes.NewReader(b))
	if e != nil {
		return nil, fmt.Errorf("invalid gzip: %w", e)
	}
	defer r.Close()
	v, e := io.ReadAll(io.LimitReader(r, MessageLimit+1))
	if e != nil {
		return nil, fmt.Errorf("truncated gzip: %w", e)
	}
	if len(v) > MessageLimit {
		return nil, fmt.Errorf("gzip output limit exceeded")
	}
	return v, nil
}
