package anomaly

// Message extraction relies only on protocol syntax already decoded by
// internal/analyze: request line, headers, query, form, JSON, multipart.
import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"gomate/internal/model"
)

type fieldKind uint8

const (
	kindValue    fieldKind = iota // enum detection, characters, shape, length
	kindEnum                      // only the set of values
	kindLen                       // only the length
	kindPresence                  // only whether the field exists
)

const valueLimit = 4096
const fieldLimit = 256

type link uint8

const (
	linkNone link = iota
	linkWeak
	linkStrong // cookies and tokens: one shared value ties sessions
)

type field struct {
	key   string
	value []byte
	size  int
	kind  fieldKind
	link  link
}

type message struct {
	incoming bool
	method   string // HTTP request: template comes from the path tree
	path     string
	tpl      string // other messages: fixed template
	summary  string
	fields   []field
	h2       string // HTTP/2 stream id, pairs requests and responses
	status   string
	interim  bool
}

func (m *message) add(key string, v []byte, kind fieldKind, l link) {
	if len(m.fields) >= fieldLimit {
		return
	}
	size := len(v)
	if len(v) > valueLimit {
		v = v[:valueLimit]
	}
	m.fields = append(m.fields, field{key: key, value: v, size: size, kind: kind, link: l})
}

func splitHead(b []byte) (string, []string, []byte) {
	head, body, ok := bytes.Cut(b, []byte("\r\n\r\n"))
	if !ok {
		head, body = b, nil
	}
	lines := strings.Split(string(head), "\r\n")
	return lines[0], lines[1:], body
}

func headerValue(lines []string, name string) string {
	for _, l := range lines {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func unescape(s string) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	if v, e := url.QueryUnescape(s); e == nil {
		return v
	}
	return s
}

func addPairs(m *message, prefix, s string, l link) {
	for _, part := range strings.Split(s, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		m.add(prefix+unescape(k), []byte(unescape(v)), kindValue, l)
	}
}

func addJSON(m *message, prefix string, v any, kind fieldKind, l link, depth int) {
	if depth > 16 || len(m.fields) >= fieldLimit {
		return
	}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			addJSON(m, prefix+"."+k, x[k], kind, l, depth+1)
		}
	case []any:
		for i, child := range x {
			if i >= 32 {
				break
			}
			addJSON(m, prefix+"[]", child, kind, l, depth+1)
		}
	case string:
		m.add(prefix, []byte(x), kind, l)
	case json.Number:
		m.add(prefix, []byte(x.String()), kind, l)
	case bool:
		if x {
			m.add(prefix, []byte("true"), kind, l)
		} else {
			m.add(prefix, []byte("false"), kind, l)
		}
	case nil:
		m.add(prefix, []byte("null"), kind, l)
	}
}

func parseJSON(b []byte) (any, bool) {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] != '{' && t[0] != '[' {
		return nil, false
	}
	d := json.NewDecoder(bytes.NewReader(t))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

// addBody describes a body by its content type. Request values are analysed,
// response values only contribute structure and size.
func addBody(m *message, contentType string, body []byte) {
	valueKind, l := kindValue, linkWeak
	if !m.incoming {
		valueKind, l = kindPresence, linkNone
	}
	media, params, _ := mime.ParseMediaType(contentType)
	switch {
	case len(body) == 0:
	case media == "application/x-www-form-urlencoded" && m.incoming:
		addPairs(m, "form.", string(body), l)
	case strings.HasPrefix(media, "multipart/") && params["boundary"] != "" && m.incoming:
		r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for i := 0; i < 64; i++ {
			p, e := r.NextPart()
			if e != nil {
				break
			}
			name := "multipart." + p.FormName()
			if f := p.FileName(); f != "" {
				m.add(name+".filename", []byte(f), kindValue, linkNone)
			}
			v, _ := io.ReadAll(io.LimitReader(p, valueLimit))
			m.add(name, v, kindValue, l)
		}
	default:
		if v, ok := parseJSON(body); ok {
			addJSON(m, "json", v, valueKind, l, 0)
		} else if m.incoming {
			m.add("body", body, kindValue, linkNone)
		}
	}
	if !m.incoming {
		m.add("body.size", body, kindLen, linkNone)
	}
}

func parseHTTP1(p model.Packet) message {
	m := message{incoming: p.Incoming}
	start, headers, body := splitHead(p.Content)
	if p.Incoming {
		// The target may contain spaces when the service URL-decodes requests.
		method, rest, ok := strings.Cut(start, " ")
		if i := strings.LastIndex(rest, " HTTP/"); i >= 0 {
			rest = rest[:i]
		}
		if !ok || rest == "" {
			m.tpl = "HTTP ?"
			return m
		}
		m.method = method
		target, query, _ := strings.Cut(rest, "?")
		m.path = target
		m.summary = method + " " + preview([]byte(rest), 120)
		addPairs(&m, "query.", query, linkWeak)
	} else {
		parts := strings.SplitN(start, " ", 3)
		if len(parts) >= 2 {
			m.status = parts[1]
		}
		m.interim = strings.HasPrefix(m.status, "1") && m.status != "101"
		m.add("status", []byte(m.status), kindEnum, linkNone)
	}
	for _, l := range headers {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch {
		case k == "cookie" && p.Incoming:
			for _, c := range strings.Split(v, ";") {
				name, value, _ := strings.Cut(strings.TrimSpace(c), "=")
				m.add("cookie."+name, []byte(value), kindValue, linkStrong)
			}
		case k == "set-cookie" && !p.Incoming:
			first, _, _ := strings.Cut(v, ";")
			name, value, _ := strings.Cut(first, "=")
			m.add("set-cookie."+name, []byte(value), kindLen, linkStrong)
		case k == "authorization" && p.Incoming:
			m.add("header."+k, []byte(v), kindValue, linkStrong)
		case k == "content-length":
		case p.Incoming || k == "content-type":
			m.add("header."+k, []byte(v), kindValue, linkNone)
		default:
			m.add("header."+k, nil, kindPresence, linkNone)
		}
	}
	addBody(&m, headerValue(headers, "Content-Type"), body)
	return m
}

func parseHTTP2Headers(p model.Packet) message {
	m := message{incoming: p.Incoming, h2: p.Metadata["streamId"]}
	for _, l := range strings.Split(string(p.Content), "\r\n") {
		k, v, ok := strings.Cut(l, ": ")
		if !ok {
			continue
		}
		switch k {
		case ":method":
			m.method = v
		case ":path":
			target, query, _ := strings.Cut(v, "?")
			m.path = target
			addPairs(&m, "query.", query, linkWeak)
		case ":status":
			m.status = v
			m.add("status", []byte(v), kindEnum, linkNone)
		case ":authority", ":scheme":
			m.add("header."+k, []byte(v), kindValue, linkNone)
		case "cookie":
			for _, c := range strings.Split(v, ";") {
				name, value, _ := strings.Cut(strings.TrimSpace(c), "=")
				m.add("cookie."+name, []byte(value), kindValue, linkStrong)
			}
		case "authorization":
			m.add("header."+k, []byte(v), kindValue, linkStrong)
		case "content-length":
		default:
			if p.Incoming || k == "content-type" || k == "grpc-status" {
				m.add("header."+k, []byte(v), kindValue, linkNone)
			} else {
				m.add("header."+k, nil, kindPresence, linkNone)
			}
		}
	}
	if m.method != "" {
		m.summary = m.method + " " + preview([]byte(p.Metadata["path"]), 120)
	}
	return m
}

// Tokens split on any non-alphanumeric byte; the whole message keeps its
// separators in the "msg" field, where injected characters show up.
func parseText(p model.Packet, prefix string, verbs func(string) string) message {
	m := message{incoming: p.Incoming}
	content := p.Content
	if len(content) > valueLimit {
		content = content[:valueLimit]
	}
	tokens := bytes.FieldsFunc(content, func(r rune) bool { return r >= 0x80 || groupOf[byte(r)] > 2 })
	verb := "*"
	if len(tokens) > 0 && len(tokens[0]) <= 16 && bytes.HasPrefix(bytes.TrimLeft(content, " \t\r\n"), tokens[0]) {
		verb = verbs(string(tokens[0]))
	}
	dir := ""
	if !p.Incoming {
		dir = " ⇐"
	}
	m.tpl = prefix + " " + verb + dir
	m.summary = prefix + " " + preview(content, 120)
	if p.Incoming {
		m.add("msg", p.Content, kindValue, linkNone)
		for i, t := range tokens {
			if i >= 32 {
				break
			}
			l := linkNone
			if len(t) >= 4 {
				l = linkWeak
			}
			m.add("tok["+strconv.Itoa(i)+"]", t, kindValue, l)
		}
	} else {
		m.add("msg", p.Content, kindLen, linkNone)
	}
	return m
}
