package api

import (
	"gomate/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMonitorProxyValidationAndAuth(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/monitor" || r.URL.Query().Get("minutes") != "60" || r.URL.Query().Get("container") != strings.Repeat("a", 64) || r.Header.Get("Authorization") != "Bearer monitor-test-token" {
			t.Errorf("wrong proxy request %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sampledAt":42,"containers":[]}`))
	}))
	defer upstream.Close()
	s := &Server{Config: config.Config{Login: "test", Password: "secret", DiscoveryURL: upstream.URL, DiscoveryToken: "monitor-test-token"}}
	for _, tc := range []struct {
		method, path string
		auth         bool
		status       int
	}{
		{"GET", "/api/v2/monitor", false, 401},
		{"POST", "/api/v2/monitor", true, 405},
		{"GET", "/api/v2/monitor?minutes=1", true, 400},
		{"GET", "/api/v2/monitor?container=../test", true, 400},
		{"GET", "/api/v2/monitor?minutes=60&container=" + strings.Repeat("a", 64), true, 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth {
			r.SetBasicAuth("test", "secret")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.status == 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("metrics must not be cached")
		}
	}
	if calls != 1 {
		t.Fatalf("invalid requests reached Docker: %d", calls)
	}
}
