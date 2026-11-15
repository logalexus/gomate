package model

import (
	"strings"
	"testing"
)

func TestTrafficSearchLiteralAndRegex(t *testing.T) {
	for _, tc := range []struct {
		query, mode, content string
		match                bool
	}{
		{"login.php?x=1", "text", "GET /LOGIN.php?x=1 HTTP/1.1", true},
		{"a.*b", "text", "a123b", false},
		{"a.*b", "regex", "A123B", true},
		{"(?-i)Secret", "regex", "secret", false},
		{"флаг", "text", "ФЛАГ", true},
		{"token=[A-Z0-9]{3}", "regex", "\x00token=AB1\xff", true},
	} {
		r, err := (TrafficSearch{Query: tc.query, Mode: tc.mode}).Compile()
		if err != nil || r.MatchString(tc.content) != tc.match {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestTrafficSearchValidation(t *testing.T) {
	before := int64(8)
	for _, search := range []TrafficSearch{
		{}, {Query: strings.Repeat("a", 513)}, {Query: "[", Mode: "regex"}, {Query: "(?=a)", Mode: "regex"},
		{Query: "x", Mode: "unknown"}, {Query: "x", Port: -1}, {Query: "x", Port: 65536},
		{Query: "x", Cursor: &SearchCursor{AfterPacket: 1}},
		{Query: "x", Cursor: &SearchCursor{Before: &before, InStream: 6, AfterPacket: 1}},
	} {
		if _, err := search.Compile(); err == nil {
			t.Fatalf("accepted invalid search: %+v", search)
		}
	}
}
