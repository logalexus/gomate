package analyze

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"gomate/internal/model"
	"strings"
	"testing"
)

func TestHTTPGzipPipeliningAndSplit(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write([]byte("FLAG_hidden"))
	w.Close()
	data := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", gz.Len()))
	data = append(data, gz.Bytes()...)
	data = append(data, []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nnext")...)
	for _, step := range []int{1, 7, 4096} {
		var d Decoder
		out := []model.Packet{}
		for i := 0; i < len(data); i += step {
			end := min(len(data), i+step)
			out = append(out, d.Feed(model.Packet{Content: data[i:end]}, model.Service{HTTP: true}, false)...)
		}
		if len(out) != 2 || !out[0].Ungzipped || !bytes.Contains(out[0].Content, []byte("FLAG_hidden")) || !bytes.HasSuffix(out[1].Content, []byte("next")) {
			t.Fatalf("step %d: %+v", step, out)
		}
	}
}
func TestHTTPLargeBodyChunkedAndURL(t *testing.T) {
	body := strings.Repeat("x", 10000)
	b := []byte(fmt.Sprintf("POST /a%%20b HTTP/1.1\r\nHost: lab\r\nContent-Length: %d\r\n\r\n%sGET /next HTTP/1.1\r\nHost: lab\r\n\r\n", len(body), body))
	var d Decoder
	out := d.Feed(model.Packet{Incoming: true, Content: b}, model.Service{HTTP: true, URLDecode: true}, false)
	if len(out) != 2 || !bytes.Contains(out[0].Content, []byte("POST /a b")) || !bytes.HasSuffix(out[0].Content, []byte(body)) {
		t.Fatalf("bad pipelined body: %d", len(out))
	}
	if out[0].Metadata["path"] != "/a%20b" || out[0].Metadata["method"] != "POST" || out[1].Metadata["path"] != "/next" {
		t.Fatalf("request metadata lost raw URI or method: %+v", out)
	}
	var c Decoder
	out = c.Feed(model.Packet{Content: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nFLAG\r\n0\r\n\r\n")}, model.Service{HTTP: true}, false)
	if len(out) != 1 || !bytes.HasSuffix(out[0].Content, []byte("FLAG")) {
		t.Fatal(out)
	}
}
func TestWebSocketMaskedFragmentedControl(t *testing.T) {
	var d Decoder
	svc := model.Service{HTTP: true, WebSockets: true}
	d.Feed(model.Packet{Incoming: true, Content: []byte("GET / HTTP/1.1\r\nHost: lab\r\nUpgrade: websocket\r\n\r\n")}, svc, false)
	mask := func(op byte, s string) []byte {
		b := []byte{op, byte(len(s)) | 128, 1, 2, 3, 4}
		for i, c := range []byte(s) {
			b = append(b, c^byte(i%4+1))
		}
		return b
	}
	b := append(mask(1, "FL"), mask(0x89, "ping")...)
	b = append(b, mask(0x80, "AG")...)
	out := d.Feed(model.Packet{Incoming: true, Content: b}, svc, false)
	if len(out) != 2 || out[0].Kind != "websocket-control" || string(out[1].Content) != "FLAG" || !out[1].WebSocketParsed {
		t.Fatal(out)
	}
}
func TestH2MultiplexedGRPCSplitCompressionAndTrailers(t *testing.T) {
	var wire bytes.Buffer
	wire.WriteString(h2Preface)
	fr := http2.NewFramer(&wire, nil)
	fr.WriteSettings()
	var hb bytes.Buffer
	enc := hpack.NewEncoder(&hb)
	for _, id := range []uint32{1, 3} {
		hb.Reset()
		for _, h := range []hpack.HeaderField{{Name: ":method", Value: "POST"}, {Name: ":path", Value: fmt.Sprint("/Echo/", id)}, {Name: "content-type", Value: "application/grpc"}, {Name: "grpc-encoding", Value: "gzip"}} {
			enc.WriteField(h)
		}
		b := append([]byte(nil), hb.Bytes()...)
		fr.WriteHeaders(http2.HeadersFrameParam{StreamID: id, BlockFragment: b[:2], EndHeaders: false})
		fr.WriteContinuation(id, true, b[2:])
	}
	envelope := func(s string, compressed bool) []byte {
		b := []byte(s)
		flag := byte(0)
		if compressed {
			var z bytes.Buffer
			w := gzip.NewWriter(&z)
			w.Write(b)
			w.Close()
			b = z.Bytes()
			flag = 1
		}
		v := make([]byte, 5)
		v[0] = flag
		binary.BigEndian.PutUint32(v[1:], uint32(len(b)))
		return append(v, b...)
	}
	a := envelope("FLAG_one", false)
	b := envelope("FLAG_two", true)
	fr.WriteData(1, false, a[:7])
	fr.WriteData(3, true, b)
	fr.WriteData(1, true, a[7:])
	for _, step := range []int{1, 11, 5000} {
		var d Decoder
		var out []model.Packet
		raw := wire.Bytes()
		for i := 0; i < len(raw); i += step {
			out = append(out, d.Feed(model.Packet{Incoming: true, Content: raw[i:min(i+step, len(raw))]}, model.Service{HTTP: true}, false)...)
		}
		if d.Requests != 2 {
			t.Fatalf("step %d: got %d requests, want 2", step, d.Requests)
		}
		seen := map[string]string{}
		for _, q := range out {
			if q.Kind == "decode-error" {
				t.Fatalf("%d: %s", step, q.Content)
			}
			if q.Kind == "grpc" || q.Kind == "http2-headers" {
				if q.Metadata["method"] != "POST" || q.Metadata["path"] != "/Echo/"+q.Metadata["streamId"] {
					t.Fatalf("missing H2 request metadata: %+v", q.Metadata)
				}
			}
			if q.Kind == "grpc" {
				seen[q.Metadata["streamId"]] = string(q.Content)
			}
		}
		if seen["1"] != "FLAG_one" || seen["3"] != "FLAG_two" {
			t.Fatal(step, seen)
		}
	}
}
func TestDecoderBoundsAndMalformed(t *testing.T) {
	var d Decoder
	out := d.Feed(model.Packet{Content: bytes.Repeat([]byte("x"), MessageLimit+1)}, model.Service{HTTP: true}, false)
	if len(out) != 1 || out[0].Kind != "decode-error" || !out[0].Incomplete || d.Size() > 1024 {
		t.Fatal(out, d.Size())
	}
	var gz bytes.Buffer
	z := gzip.NewWriter(&gz)
	z.Write(bytes.Repeat([]byte("a"), MessageLimit+1))
	z.Close()
	if _, err := gunzip(gz.Bytes()); err == nil {
		t.Fatal("gzip bomb accepted")
	}
}
func TestStreamingMatchAcrossPublications(t *testing.T) {
	c := NewCatalog()
	p := model.Pattern{ID: 1, Name: "flag", Value: "FLAG", Enabled: true, Color: "#112233", SearchType: "SUBSTRING", Direction: "BOTH", Action: "FIND"}
	if e := c.Replace(nil, []model.Pattern{p}); e != nil {
		t.Fatal(e)
	}
	state := MatchState{}
	a := model.Chunk{Stream: model.Stream{Protocol: "TCP"}, Packets: []model.Packet{{Content: []byte("hello FL"), Incoming: true}, {Content: []byte("unrelated"), Incoming: false}}}
	c.AnalyzeStreaming(&a, &state)
	b := model.Chunk{Stream: model.Stream{Protocol: "TCP"}, Packets: []model.Packet{{Content: []byte("AG there"), Incoming: true}}}
	c.AnalyzeStreaming(&b, &state)
	if len(b.Stream.Patterns) != 1 || len(b.Packets[0].Matches) != 1 || !b.Packets[0].Matches[0].CrossBoundary || b.Packets[0].Matches[0].End != 1 {
		t.Fatalf("%+v", b)
	}
	state = MatchState{}
	c.AnalyzeStreaming(&a, &state)
	b.Packets[0].Incomplete = true
	c.AnalyzeStreaming(&b, &state)
	if len(b.Stream.Patterns) != 0 {
		t.Fatal("matched across missing data")
	}
}
func TestProcessorBudgetAndRelease(t *testing.T) {
	cat := NewCatalog()
	cat.Replace(nil, []model.Pattern{{ID: 1, Name: "regex", Value: "FLAG.*", Enabled: true, Color: "#112233", SearchType: "REGEX", Direction: "BOTH", Action: "FIND"}})
	p := NewProcessor(cat)
	p.MaxBytes = 4096
	for i := 0; i < 100; i++ {
		c := model.Chunk{Stream: model.Stream{Key: fmt.Sprint(i), Protocol: "TCP"}, Packets: []model.Packet{{Content: make([]byte, 8192)}}}
		p.Process(&c)
		if p.Size() > 4096 {
			t.Fatal(p.Size())
		}
	}
	if p.Evictions != 100 {
		t.Fatal(p.Evictions)
	}
}
func FuzzProtocolDecoder(f *testing.F) {
	f.Add([]byte("GET / HTTP/1.1\r\nHost: lab\r\n\r\n"))
	f.Add([]byte(h2Preface))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MessageLimit {
			return
		}
		var d Decoder
		svc := model.Service{HTTP: true, WebSockets: true, URLDecode: true}
		for i := 0; i < len(b); i += 127 {
			d.Feed(model.Packet{Content: b[i:min(i+127, len(b))], Incoming: true}, svc, false)
		}
		d.Feed(model.Packet{Incoming: true}, svc, true)
	})
}

func TestHTTP2BodyAcrossFrames(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(h2Preface)
	fr := http2.NewFramer(&b, nil)
	fr.WriteSettings()
	var hb bytes.Buffer
	enc := hpack.NewEncoder(&hb)
	enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
	enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/test"})
	fr.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: hb.Bytes(), EndHeaders: true})
	fr.WriteData(1, false, []byte("FL"))
	fr.WriteData(1, true, []byte("AG"))
	var d Decoder
	out := d.Feed(model.Packet{Incoming: true, Content: b.Bytes()}, model.Service{HTTP: true}, false)
	found := false
	for _, p := range out {
		if p.Kind == "http2-body" && string(p.Content) == "FLAG" {
			found = true
		}
	}
	if !found {
		t.Fatal(out)
	}
}

func TestProcessorCountsRequestsAcrossChunksAndGaps(t *testing.T) {
	cat := NewCatalog()
	if err := cat.Replace([]model.Service{{Port: 80, HTTP: true}}, nil); err != nil {
		t.Fatal(err)
	}
	p := NewProcessor(cat)
	sequence := int64(0)
	feed := func(data string, incoming, gap, closed bool) {
		sequence++
		c := model.Chunk{Sequence: sequence, Closed: closed, Stream: model.Stream{Key: "keepalive", Service: 80, Protocol: "TCP"}, Packets: []model.Packet{{Content: []byte(data), Incoming: incoming, Incomplete: gap}}}
		p.Process(&c)
	}
	feed("POST /a HTTP/1.1\r\nHost: lab\r\nContent-Length: 4\r\n\r\nab", true, false, false)
	if p.Requests != 0 {
		t.Fatal("counted incomplete request")
	}
	feed("cdGET /b HTTP/1.1\r\nHost: lab\r\n\r\n", true, false, false)
	if p.Requests != 2 {
		t.Fatalf("got %d requests, want 2", p.Requests)
	}
	feed("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n", false, false, false)
	if p.Requests != 2 {
		t.Fatal("counted responses")
	}
	feed("GET /c HTTP/1.1\r\nHost: lab\r\n\r\n", true, true, true)
	if p.Requests != 3 {
		t.Fatalf("counter lost across gap/close: %d", p.Requests)
	}
}

func TestH2RequestCountExcludesTrailersAndResponses(t *testing.T) {
	var wire, hb bytes.Buffer
	wire.WriteString(h2Preface)
	fr := http2.NewFramer(&wire, nil)
	enc := hpack.NewEncoder(&hb)
	headers := func(fields []hpack.HeaderField, end bool) {
		hb.Reset()
		for _, h := range fields {
			if err := enc.WriteField(h); err != nil {
				t.Fatal(err)
			}
		}
		if err := fr.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: hb.Bytes(), EndHeaders: true, EndStream: end}); err != nil {
			t.Fatal(err)
		}
	}
	headers([]hpack.HeaderField{{Name: ":method", Value: "POST"}, {Name: ":path", Value: "/Echo"}, {Name: "content-type", Value: "application/grpc"}}, false)
	// Two gRPC messages are one HTTP/2 request.
	fr.WriteData(1, false, []byte{0, 0, 0, 0, 1, 'a', 0, 0, 0, 0, 1, 'b'})
	headers([]hpack.HeaderField{{Name: "trailer", Value: "value"}}, true)
	var d Decoder
	out := d.Feed(model.Packet{Incoming: true, Content: wire.Bytes()}, model.Service{HTTP: true}, false)
	for _, p := range out {
		if p.Kind == "decode-error" {
			t.Fatal(string(p.Content))
		}
	}
	if d.Requests != 1 {
		t.Fatalf("trailers/messages double counted: %d", d.Requests)
	}
	wire.Reset()
	hb.Reset()
	enc = hpack.NewEncoder(&hb)
	fr.WriteSettings()
	headers([]hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "grpc-status", Value: "0"}}, true)
	d.Feed(model.Packet{Content: wire.Bytes()}, model.Service{HTTP: true}, false)
	if d.Requests != 1 {
		t.Fatal("counted response headers")
	}
}
