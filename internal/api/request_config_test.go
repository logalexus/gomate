package api

import (
	"context"
	"encoding/json"
	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/control"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"net/http/httptest"
	"testing"
)

func TestRequestTimeoutConfigWithoutDatabase(t *testing.T) {
	cfg := config.Config{Login: "test", Password: "test"}
	s := &Server{Config: cfg}
	call := func(auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v2/config", nil)
		if auth {
			r.SetBasicAuth("test", "test")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := call(false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	read := func(want int64) {
		t.Helper()
		w := call(true)
		var got struct {
			Seconds int64 `json:"timeoutSeconds"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Seconds != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	read(10) // PCAP mode and older settings use the same default.
	s.Control = control.New(context.Background(), cfg, analyze.NewCatalog(), &metrics.Metrics{}, nil, func() {})
	v := model.RuntimeSettings{CleanupIntervalSeconds: 5, RequestTimeoutSeconds: 60}
	if err := s.Control.Apply(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	read(60)
	// A saved anomaly-only setting migrates to the common application budget.
	legacy := model.RuntimeSettings{CleanupIntervalSeconds: 5, AnomalyTimeoutSeconds: 45}
	if err := s.Control.Apply(context.Background(), legacy, nil); err != nil {
		t.Fatal(err)
	}
	read(45)
	if err := s.Control.Apply(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{-1, 4, 301} {
		v.RequestTimeoutSeconds = n
		if err := s.Control.Apply(context.Background(), v, nil); err == nil {
			t.Fatalf("accepted %d", n)
		}
		read(60)
	}
	v.RequestTimeoutSeconds = 5
	if err := s.Control.Apply(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	read(5)
}
