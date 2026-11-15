package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/model"
)

func TestTrafficSearchContentFiltersAndContinuation(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	save := func(key string, port int, patterns []int, packets ...model.Packet) {
		t.Helper()
		if err := s.SaveChunks(ctx, []*model.Chunk{{Token: key, Stream: model.Stream{Key: key, Service: port, Protocol: "TCP", Patterns: patterns}, Packets: packets}}); err != nil {
			t.Fatal(err)
		}
	}
	save("old", 80, []int{1, 2}, model.Packet{Content: []byte("POST /api/login.php?x=1 HTTP/1.1\x00Secret=ABC123")})
	for i := 0; i < 140; i++ {
		save(fmt.Sprint(i), 81, []int{1}, model.Packet{Content: []byte("noise")})
	}
	large := make([]model.Packet, 700)
	for i := range large {
		large[i].Content = []byte("noise")
	}
	large[699].Content = []byte("late-match token=AB1")
	save("large", 80, []int{1, 2}, large...)
	if err := s.Favorite(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s}
	request := func(v model.TrafficSearch) (model.TrafficSearchResult, int) {
		t.Helper()
		body, _ := json.Marshal(v)
		r := httptest.NewRequest("POST", "/api/v2/traffic/search", strings.NewReader(string(body)))
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		var result model.TrafficSearchResult
		if w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
		}
		return result, w.Code
	}
	search := model.TrafficSearch{Page: model.Page{PageSize: 25}, Query: "late-match", Mode: "text"}
	first, code := request(search)
	if code != 200 || len(first.Streams) != 0 || first.Next == nil || first.Next.InStream != 142 || first.Next.AfterPacket == 0 {
		t.Fatalf("large session did not yield: %+v status %d", first, code)
	}
	search.Cursor = first.Next
	next, code := request(search)
	if code != 200 || len(next.Streams) != 1 || next.Streams[0].ID != 142 {
		t.Fatalf("continuation lost match: %+v status %d", next, code)
	}
	search = model.TrafficSearch{Page: model.Page{PageSize: 1, PatternIDs: []int{1, 2}, Favorites: true}, Query: "/API/login.php?x=1", Port: 80}
	result, code := request(search)
	if code != 200 || len(result.Streams) != 1 || result.Streams[0].ID != 1 {
		t.Fatalf("combined filters failed: %+v %d", result, code)
	}
	search.Query = `secret=[a-z]{3}\d{3}`
	search.Mode = "regex"
	result, code = request(search)
	if code != 200 || len(result.Streams) != 1 {
		t.Fatalf("regex on binary content failed: %+v %d", result, code)
	}
	search.Query = "Secret=ABC123"
	search.Mode = "text"
	search.Port = 81
	result, code = request(search)
	if code != 200 || len(result.Streams) != 0 {
		t.Fatal("service filter bypassed", result, code)
	}
	// Search beyond a whole candidate batch, without returning duplicate sessions.
	search = model.TrafficSearch{Page: model.Page{PageSize: 25}, Query: "Secret=ABC123"}
	seen := map[int64]bool{}
	found := false
	for i := 0; i < 10; i++ {
		result, code = request(search)
		if code != 200 {
			t.Fatal(code)
		}
		for _, stream := range result.Streams {
			if seen[stream.ID] {
				t.Fatal("duplicate", stream.ID)
			}
			seen[stream.ID] = true
			if stream.ID == 1 {
				found = true
			}
		}
		if result.Next == nil {
			break
		}
		search.Cursor = result.Next
	}
	if !found {
		t.Fatal("old match beyond candidate window not found")
	}
	for _, v := range []model.TrafficSearch{{Query: "[", Mode: "regex"}, {Query: strings.Repeat("x", 513)}, {Query: "x", Mode: "bad"}} {
		if _, code = request(v); code != 400 {
			t.Fatal("invalid search accepted", code)
		}
	}
	// Metadata remains searchable after payload removal.
	if _, err := s.Pool.Exec(ctx, `UPDATE gomate_streams SET first_request_path='/retained/path',payload_purged=true WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM gomate_packets WHERE stream_id=1`); err != nil {
		t.Fatal(err)
	}
	result, code = request(model.TrafficSearch{Page: model.Page{PageSize: 25, Favorites: true}, Query: "/retained/path", Port: 80})
	if code != 200 || len(result.Streams) != 1 || result.Streams[0].ID != 1 {
		t.Fatal("retained metadata not found", result, code)
	}
}

func TestTrafficSearchYieldsAtPayloadBudget(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	packets := make([]model.Packet, 6)
	for i := range packets {
		packets[i].Content = []byte(strings.Repeat("x", 3<<20))
	}
	packets[5].Content = []byte("last-packet")
	if err := s.SaveChunks(ctx, []*model.Chunk{{Token: "large-payload", Stream: model.Stream{Key: "large-payload", Service: 80, Protocol: "TCP"}, Packets: packets}}); err != nil {
		t.Fatal(err)
	}
	search := model.TrafficSearch{Page: model.Page{PageSize: 25}, Query: "last-packet"}
	re, err := search.Compile()
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SearchTraffic(ctx, search, re)
	if err != nil || len(first.Streams) != 0 || first.Next == nil || first.Next.InStream != 1 || first.Next.AfterPacket > 3 {
		t.Fatal("payload budget did not yield", first, err)
	}
	search.Cursor = first.Next
	found := false
	for i := 0; i < 6; i++ {
		next, err := s.SearchTraffic(ctx, search, re)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Streams) == 1 {
			found = true
			break
		}
		if next.Next == nil {
			break
		}
		search.Cursor = next.Next
	}
	if !found {
		t.Fatal("payload continuation skipped remaining packets")
	}
}
