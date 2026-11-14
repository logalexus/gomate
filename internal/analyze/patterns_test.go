package analyze

import (
	"gomate/internal/model"
	"sync"
	"testing"
)

func pattern(id int, kind, value string) model.Pattern {
	return model.Pattern{ID: id, Name: "test", Value: value, Color: "#ff0000", SearchType: kind, Direction: "BOTH", Action: "FIND", Enabled: true}
}
func TestPatternsUnicodeBinaryAndZeroLength(t *testing.T) {
	c := NewCatalog()
	if err := c.Replace(nil, []model.Pattern{pattern(1, "REGEX", `^|FLAG`), pattern(2, "SUBSTRING", "FLAG"), pattern(3, "SUBBYTES", "ff00")}); err != nil {
		t.Fatal(err)
	}
	chunk := &model.Chunk{Packets: []model.Packet{{Content: []byte("😀FLAG")}, {Content: []byte{255, 0}}}}
	c.Analyze(chunk)
	if len(chunk.Packets[0].Matches) != 2 {
		t.Fatalf("matches: %+v", chunk.Packets[0].Matches)
	}
	for _, m := range chunk.Packets[0].Matches {
		if m.Start != 2 || m.End != 5 {
			t.Fatalf("UTF-16 offsets: %+v", m)
		}
	}
	if got := chunk.Packets[1].Matches; len(got) != 1 || got[0].Start != 0 || got[0].End != 1 {
		t.Fatalf("binary offsets: %+v", got)
	}
}
func TestDirectionServiceAndIgnore(t *testing.T) {
	c := NewCatalog()
	p := pattern(1, "SUBSTRING", "secret")
	p.Action = "IGNORE"
	p.Direction = "OUTPUT"
	port := 80
	p.ServiceID = &port
	c.Replace(nil, []model.Pattern{p})
	for _, tc := range []struct {
		port           int
		incoming, want bool
	}{{80, false, true}, {80, true, false}, {81, false, false}} {
		v := &model.Chunk{Stream: model.Stream{Service: tc.port}, Packets: []model.Packet{{Content: []byte("secret"), Incoming: tc.incoming}}}
		c.Analyze(v)
		if v.Ignore != tc.want {
			t.Fatalf("%+v: ignore=%v", tc, v.Ignore)
		}
	}
}
func TestConcurrentCatalogReplacement(t *testing.T) {
	c := NewCatalog()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 100; j++ {
				if err := c.Replace([]model.Service{{Port: 80}}, []model.Pattern{pattern(1, "REGEX", "a*")}); err != nil {
					t.Error(err)
				}
				c.Service(80)
				c.Analyze(&model.Chunk{Packets: []model.Packet{{Content: []byte("aaaaab")}}})
			}
		})
	}
	wg.Wait()
}
func TestRejectUnsupportedRegex(t *testing.T) {
	if err := Validate(pattern(1, "REGEX", `(?<=a)b`)); err == nil {
		t.Fatal("lookbehind accepted")
	}
}
