package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gomate/internal/analyze"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

func TestStreamMultiPatternAPIAndCursor(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	for i, ids := range [][]int{{1}, {2}, {1, 2}, {1, 2, 3}, {1, 2}} {
		port := 80
		if i == 4 {
			port = 81
		}
		chunk := &model.Chunk{Token: fmt.Sprint(i), Stream: model.Stream{Key: fmt.Sprint(i), Service: port, Protocol: "TCP", Patterns: ids}, Packets: []model.Packet{{Content: []byte("test")}}}
		if err := s.SaveChunks(ctx, []*model.Chunk{chunk}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Favorite(ctx, 4, true); err != nil {
		t.Fatal(err)
	}
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s, Catalog: analyze.NewCatalog(), Metrics: &metrics.Metrics{}, Hub: events.New()}
	request := func(port, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/stream/"+port, strings.NewReader(body))
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	cases := []struct {
		port, body string
		ids        []int64
	}{
		{"all", `{"patternIds":[1,2],"pageSize":1}`, []int64{5}},
		{"all", `{"patternIds":[1,2],"pageSize":1,"startingFrom":5}`, []int64{4}},
		{"80", `{"patternIds":[1,2]}`, []int64{4, 3}},
		{"80", `{"patternIds":[1,2],"favorites":true}`, []int64{4}},
		{"all", `{"patternIds":[1,2,2],"pattern":{"id":"3"}}`, []int64{4}},
		{"80", `{"pattern":{"id":"2"}}`, []int64{4, 3, 2}},
		{"all", `{"patternIds":[],"pattern":{"id":0}}`, []int64{5, 4, 3, 2, 1}},
		{"all", `{"patternIds":[999]}`, []int64{}},
	}
	for _, tc := range cases {
		w := request(tc.port, tc.body)
		if w.Code != 200 {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		var rows []model.Stream
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(tc.ids) {
			t.Fatalf("%s: %+v", tc.body, rows)
		}
		for i, id := range tc.ids {
			if rows[i].ID != id {
				t.Fatal(tc, rows)
			}
		}
	}
	for _, body := range []string{`{"patternIds":[0]}`, `{"patternIds":[-2]}`, `{"patternIds":[1.5]}`, `{"patternIds":["1"]}`, `{"patternIds":[2147483648]}`, `{"pattern":{"id":-1}}`, `{"patternIds":[` + strings.TrimSuffix(strings.Repeat("1,", 101), ",") + `]}`} {
		if w := request("all", body); w.Code != 400 {
			t.Fatal("invalid pattern selection accepted", body, w.Code, w.Body.String())
		}
	}
	hundred := make([]int, 100)
	for i := range hundred {
		hundred[i] = i + 1
	}
	body, _ := json.Marshal(map[string]any{"patternIds": hundred})
	if w := request("all", string(body)); w.Code != 200 {
		t.Fatal("100 patterns rejected", w.Code, w.Body.String())
	}
}

func TestFirstRequestMetadataIsStableAndSurvivesPayloadCleanup(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	packet := func(incoming bool, kind, path, method string) model.Packet {
		return model.Packet{Incoming: incoming, Kind: kind, Metadata: map[string]string{"path": path, "method": method}, Content: []byte("payload")}
	}
	save := func(token, key string, packets ...model.Packet) {
		t.Helper()
		c := &model.Chunk{Token: token, Stream: model.Stream{Key: key, Service: 80, Protocol: "TCP"}, Packets: packets}
		if err := s.SaveChunks(ctx, []*model.Chunk{c}); err != nil {
			t.Fatal(err)
		}
	}
	save("before", "http", packet(false, "http", "/response-only", ""), packet(true, "", "/raw-fake", "GET"))
	rows, err := s.Streams(ctx, 0, model.Page{PageSize: 100})
	if err != nil || len(rows) != 1 || rows[0].FirstRequestPath != "" {
		t.Fatal(rows, err)
	}
	save("first", "http", packet(false, "http", "/not-request", ""), packet(true, "http", "/first?x=%20", "POST"), packet(true, "http", "/second", "GET"))
	save("later", "http", packet(true, "http", "/third", "DELETE"))
	save("first", "http", packet(true, "http", "/duplicate-retry", "GET"))
	save("h2", "h2", packet(true, "http2-headers", "/hello", "GET"), packet(true, "grpc", "/later/rpc", "POST"))
	save("grpc", "grpc", packet(true, "grpc", "/service/Method", "POST"))
	save("unicode", "unicode", packet(true, "http", strings.Repeat("界", 1000), "GET"))
	rows, err = s.Streams(ctx, 0, model.Page{PageSize: 100})
	if err != nil || len(rows) != 4 {
		t.Fatal(rows, err)
	}
	if rows[0].FirstRequestPath == "" || len(rows[0].FirstRequestPath) > 2048 || !utf8.ValidString(rows[0].FirstRequestPath) || !rows[0].FirstRequestTruncated {
		t.Fatal("unbounded or invalid Unicode", rows[0])
	}
	if rows[1].FirstRequestPath != "/service/Method" || rows[1].FirstRequestMethod != "POST" || rows[2].FirstRequestPath != "/hello" || rows[2].FirstRequestMethod != "GET" || rows[3].FirstRequestPath != "/first?x=%20" || rows[3].FirstRequestMethod != "POST" {
		t.Fatalf("incorrect first-request metadata: %+v", rows)
	}
	expected := rows[3]
	if _, err = s.StartCleanup(ctx, "packets", false, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err = s.StepCleanup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	packets, err := s.Packets(ctx, expected.ID, model.Page{PageSize: 100})
	if err != nil || len(packets) != 0 {
		t.Fatal(packets, err)
	}
	rows, err = s.Streams(ctx, 0, model.Page{PageSize: 100})
	if err != nil || rows[3].FirstRequestPath != expected.FirstRequestPath || !rows[3].PayloadPurged {
		t.Fatal("path lost on purge", rows, err)
	}
	save("after-purge", "http", packet(true, "http", "/after-purge", "PUT"))
	rows, err = s.Streams(ctx, 0, model.Page{PageSize: 100})
	if err != nil || rows[3].FirstRequestPath != expected.FirstRequestPath {
		t.Fatal("path changed after purge", rows, err)
	}
}
