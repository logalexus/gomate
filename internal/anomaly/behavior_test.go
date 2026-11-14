package anomaly

import (
	"encoding/json"
	"strings"
	"testing"

	"gomate/internal/model"
)

func TestBehaviorCanonicalStructure(t *testing.T) {
	extract := func(body string) BehaviorMessage {
		b, ok := BehaviorPacket(model.Packet{Kind: "http", Incoming: true, Content: []byte("POST /exec HTTP/1.1\r\nContent-Type: application/json\r\n\r\n" + body)})
		if !ok {
			t.Fatal("missing descriptor")
		}
		return b
	}
	a := extract(`{"command":"id","data":{"x":true,"n":1},"empty":{}}`)
	b := extract(`{"empty":{},"data":{"n":1,"x":true},"command":"id"}`)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Fatal("JSON ordering split identical requests")
	}
	shape := func(v BehaviorMessage) string {
		for _, f := range v.Fields {
			if f.Key == "json.structure" {
				return f.Value
			}
		}
		return ""
	}
	for _, body := range []string{`{"command":"id","data":{"x":"true","n":1},"empty":{}}`, `{"command":"id","data":{"x":true,"n":1},"empty":[]}`} {
		if shape(a) == shape(extract(body)) {
			t.Fatal("JSON structure lost", body)
		}
	}
	large := extract(`{"x":"` + strings.Repeat("a", 5000) + `"}`)
	if !large.Limited {
		t.Fatal("missing truncation indication")
	}
	if _, ok := BehaviorPacket(model.Packet{Incoming: false, Content: []byte("reply")}); ok {
		t.Fatal("server response treated as client behavior")
	}
}
