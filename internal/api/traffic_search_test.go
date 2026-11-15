package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"gomate/internal/config"
)

func TestTrafficSearchRejectsOverloadBeforeDatabaseAccess(t *testing.T) {
	s := &Server{Config: config.Config{Login: "test", Password: "test"}}
	s.trafficSearchRunning.Store(2)
	r := httptest.NewRequest("POST", "/api/v2/traffic/search", strings.NewReader(`{"query":"hello"}`))
	r.SetBasicAuth("test", "test")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" || s.trafficSearchRunning.Load() != 2 {
		t.Fatal(w.Code, w.Body.String(), s.trafficSearchRunning.Load())
	}
}
