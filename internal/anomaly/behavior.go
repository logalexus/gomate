package anomaly

// Behavior descriptors are independent of anomaly scoring and learning. They
// describe a bounded prefix of decoded client messages, not attack attribution.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gomate/internal/model"
)

const BehaviorMessages = 8
const behaviorFields = 48

type BehaviorField struct {
	Key     string `json:"k"`
	Value   string `json:"v"` // digest; equality never depends on the display preview
	Preview string `json:"p"`
}
type BehaviorMessage struct {
	Kind    string          `json:"kind"`
	Fields  []BehaviorField `json:"fields"`
	Limited bool            `json:"limited,omitempty"`
}

func BehaviorPacket(p model.Packet) (BehaviorMessage, bool) {
	out := BehaviorMessage{Kind: p.Kind, Fields: []BehaviorField{}}
	if !p.Incoming || len(p.Content) == 0 {
		return out, false
	}
	if len(p.Content) > 65536 {
		p.Content = p.Content[:65536]
		out.Limited = true
	}
	var m message
	switch p.Kind {
	case "http":
		m = parseHTTP1(p)
	case "http2-headers":
		m = parseHTTP2Headers(p)
	case "http2-body", "grpc", "websocket", "":
		m = message{incoming: true}
		if v, ok := parseJSON(p.Content); ok {
			addJSON(&m, "json", v, kindValue, linkNone, 0)
		} else {
			// Keep separators and token positions, but allow changing individual tokens.
			// No UUID/JWT/hex rules and no dependence on source addresses or labels.
			var structure strings.Builder
			var token strings.Builder
			flush := func() {
				if token.Len() > 0 {
					m.add("token."+strconv.Itoa(len(m.fields)), []byte(token.String()), kindValue, linkNone)
					token.Reset()
					structure.WriteByte('*')
				}
			}
			for _, r := range string(p.Content) {
				if unicode.IsLetter(r) || unicode.IsNumber(r) {
					token.WriteRune(r)
				} else {
					flush()
					structure.WriteRune(r)
				}
			}
			flush()
			m.add("separators", []byte(structure.String()), kindValue, linkNone)
		}
	default:
		return out, false
	}
	if m.method != "" {
		m.add("method", []byte(m.method), kindEnum, linkNone)
	}
	if m.path != "" {
		m.add("path", []byte(m.path), kindEnum, linkNone)
	}
	// Preserve JSON types, empty containers and nesting independently of values.
	body := p.Content
	if p.Kind == "http" {
		_, _, body = splitHead(p.Content)
	}
	if v, ok := parseJSON(body); ok {
		m.add("json.structure", []byte(behaviorJSONShape(v, 0)), kindEnum, linkNone)
	}
	// Parsing map order must not influence field order or truncation.
	sort.SliceStable(m.fields, func(i, j int) bool { return m.fields[i].key < m.fields[j].key })
	if len(m.fields) >= fieldLimit {
		out.Limited = true
	}
	occurrences := map[string]int{}
	for _, f := range m.fields {
		if len(out.Fields) >= behaviorFields {
			out.Limited = true
			break
		}
		k := f.key
		occurrences[k]++
		if occurrences[k] > 1 {
			k += "#" + strconv.Itoa(occurrences[k])
		}
		sum := sha256.Sum256(f.value)
		v := hex.EncodeToString(sum[:16]) + ":" + strconv.Itoa(f.size)
		display := preview(f.value, 80)
		if f.key == "json.structure" {
			display = "JSON structure"
		}
		if f.link == linkStrong {
			display = "[value hidden]"
		}
		if f.size > valueLimit {
			out.Limited = true
		}
		if len(k) > 256 {
			k = k[:256]
			out.Limited = true
		}
		out.Fields = append(out.Fields, BehaviorField{Key: k, Value: v, Preview: display})
	}
	return out, true
}

func behaviorJSONShape(v any, depth int) string {
	if depth > 16 {
		return "…"
	}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for i, k := range keys {
			if i >= 48 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, strconv.Quote(k)+":"+behaviorJSONShape(x[k], depth+1))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := []string{}
		for i, v := range x {
			if i >= 32 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, behaviorJSONShape(v, depth+1))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "bool"
	default:
		return "null"
	}
}
