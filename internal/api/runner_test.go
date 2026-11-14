package api

import (
	"encoding/json"
	"gomate/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRunnerProxyAuthFixedPathsAndDBIndependence(t *testing.T) {
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer gateway-test-token" {
			t.Error("missing gateway authorization")
		}
		if r.URL.Path != "/runner/jobs" {
			t.Error("wrong fixed path", r.URL.Path)
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil || body["code"] != "print(123)" {
			t.Error(body, e)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		io.WriteString(w, `{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"starting"}`)
	}))
	defer gateway.Close()
	app := &Server{Config: config.Config{Login: "test", Password: "test", DiscoveryURL: gateway.URL, DiscoveryToken: "gateway-test-token"}}
	call := func(method, path, body, origin string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.SetBasicAuth("test", "test")
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	body := `{"language":"python","code":"print(123)"}`
	if w := call("POST", "/api/v2/runner/jobs", body, "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/v2/runner/jobs", body, "https://foreign.invalid", true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/v2/runner/containers/create", body, "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized/invalid request proxied")
	}
	if w := call("POST", "/api/v2/runner/jobs", body, "", true); w.Code != 202 || !strings.Contains(w.Body.String(), "starting") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/v2/runner/jobs", strings.Repeat("x", 2<<20+1), "", true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
