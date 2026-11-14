package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"gomate/internal/storage"
)

func database(t *testing.T) *storage.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to isolated gomate_test database")
	}
	u, e := url.Parse(dsn)
	if e != nil || u.Path != "/gomate_test" {
		t.Fatal("integration tests require database named gomate_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := storage.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	_, e = s.Pool.Exec(ctx, "TRUNCATE gomate_chat,gomate_settings,gomate_cleanup_jobs,gomate_jobs,gomate_import_state,gomate_packets,gomate_streams,gomate_chunks,gomate_services,gomate_patterns RESTART IDENTITY CASCADE")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestAtomicIdempotentWritePaginationAndRetention(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	first := &model.Chunk{Token: "one", Stream: model.Stream{Key: "flow1", Service: 80, Protocol: "TCP", Start: 1, End: 2, Patterns: []int{1}}, Bytes: 4, Packets: []model.Packet{{Content: []byte("FLAG"), Matches: []model.Match{{PatternID: 1, Start: 0, End: 3}}}}}
	if e := s.SaveChunks(ctx, []*model.Chunk{first}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveChunks(ctx, []*model.Chunk{first}); e != nil {
		t.Fatal(e)
	}
	rows, e := s.Streams(ctx, 80, model.Page{PageSize: 25})
	if e != nil || len(rows) != 1 || rows[0].Size != 4 {
		t.Fatalf("%+v %v", rows, e)
	}
	id := rows[0].ID
	packets, e := s.Packets(ctx, id, model.Page{PageSize: 25})
	if e != nil || len(packets) != 1 || string(packets[0].Content) != "FLAG" {
		t.Fatalf("%+v %v", packets, e)
	}
	second := *first
	second.Token = "two"
	second.Packets = []model.Packet{{Content: []byte("more"), Matches: []model.Match{}}}
	if e = s.SaveChunks(ctx, []*model.Chunk{&second}); e != nil {
		t.Fatal(e)
	}
	after := packets[0].ID
	packets, e = s.Packets(ctx, id, model.Page{PageSize: 25, StartingFrom: &after})
	if e != nil || len(packets) != 1 || string(packets[0].Content) != "more" {
		t.Fatalf("cursor: %+v %v", packets, e)
	}
	s.Favorite(ctx, id, true)
	if e = s.Cleanup(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	rows, e = s.Streams(ctx, 0, model.Page{PageSize: 25})
	if e != nil || len(rows) != 1 {
		t.Fatal("favorite was removed", e)
	}
	s.Favorite(ctx, id, false)
	if e = s.Cleanup(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	rows, e = s.Streams(ctx, 0, model.Page{PageSize: 25})
	if e != nil || len(rows) != 0 {
		t.Fatal("retention failed", e)
	}
}
func TestAPIContract(t *testing.T) {
	s := database(t)
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s, Catalog: analyze.NewCatalog(), Metrics: &metrics.Metrics{}, Hub: events.New()}
	request := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.SetBasicAuth("test", "test")
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/service/", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/service/", `{"port":80,"name":"web"}`, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, ok := app.Catalog.Service(80); !ok {
		t.Fatal("catalog not refreshed")
	}
	w := request("POST", "/api/pattern/", `{"name":"flag","value":"FLAG","color":"#ff0000","searchType":"SUBSTRING","directionType":"BOTH","actionType":"FIND"}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var p model.Pattern
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != 1 || !p.Enabled {
		t.Fatal(p)
	}
	if w = request("POST", "/api/stream/all", `{"pageSize":1000000}`, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = request("POST", "/api/stream/all", `{"pageSize":25}`, true); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("GET", "/api/v2/snapshot", "", true); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
}
