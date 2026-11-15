package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gomate/internal/analyze"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/discovery"
	"gomate/internal/events"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

func TestDockerServiceEditableSettings(t *testing.T) {
	store := database(t)
	candidates := []discovery.Candidate{
		{ID: "http", Name: "Docker HTTP", Port: 18080, PrivatePort: 80, Available: true},
		{ID: "legacy", Name: "Docker TCP", Port: 19090, PrivatePort: 9090, Available: true},
		{ID: "unpublished", Name: "Internal", PrivatePort: 9000},
	}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("invalid discovery request")
		}
		json.NewEncoder(w).Encode(candidates)
	}))
	defer gateway.Close()
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DiscoveryURL: gateway.URL, DiscoveryToken: "secret"}, Store: store, Catalog: analyze.NewCatalog(), Metrics: &metrics.Metrics{}, Hub: events.New()}
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v2/docker-services", strings.NewReader(body))
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"id":"removed","service":{"name":"Changed"}}`, 409},
		{`{"id":"unpublished","service":{"name":"Changed","port":9000}}`, 409},
		{`{"id":"http","service":{"name":"  "}}`, 400},
		{`{"id":"http","service":{"name":"` + strings.Repeat("x", 257) + `"}}`, 400},
		{`{"id":"http","service":{"name":"TLS","decryptTls":true}}`, 422},
	} {
		if w := request(tc.body); w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	rows, err := store.Services(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("invalid creation persisted: %+v %v", rows, err)
	}
	w := request(`{"id":"http","service":{"port":9999,"name":"Edited API","http":true,"urldecodeHttpRequests":true,"parseWebSockets":true}}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var actual model.Service
	if err = json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	want := model.Service{Port: 18080, Name: "Edited API", HTTP: true, URLDecode: true, WebSockets: true}
	if actual != want {
		t.Fatalf("created %+v want %+v", actual, want)
	}
	if _, ok := app.Catalog.Service(18080); !ok {
		t.Fatal("capture catalog was not refreshed")
	}
	if _, ok := app.Catalog.Service(9999); ok {
		t.Fatal("client port was trusted")
	}
	if w = request(`{"id":"http","service":{"name":"Overwrite","http":false}}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	rows, err = store.Services(context.Background())
	if err != nil || len(rows) != 1 || rows[0] != want {
		t.Fatalf("duplicate creation overwrote service: %+v %v", rows, err)
	}
	if w = request(`{"id":"legacy"}`); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if actual != (model.Service{Port: 19090, Name: "Docker TCP"}) {
		t.Fatalf("legacy creation changed: %+v", actual)
	}
}
