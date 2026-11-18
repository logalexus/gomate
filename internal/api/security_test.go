package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/metrics"
)

func TestAllRoutesRequireBasicAuth(t *testing.T) {
	s := &Server{Config: config.Config{Login: "audit", Password: "secret"}}
	paths := []string{"/", "/index.html", "/assets/app.js", "/favicon.ico", "/health/live", "/health/ready", "/metrics", "/api/v2/config", "/api/v2/snapshot", "/api/v2/diagnostics", "/api/v2/settings", "/api/v2/events", "/api/v2/chat", "/api/v2/chat/arena", "/api/v2/monitor", "/api/v2/containers", "/api/v2/containers/id/start", "/api/v2/docker-services", "/api/v2/runner/jobs", "/api/v2/farm/client.py", "/api/v2/farm/client-credentials", "/api/v2/farm/flags", "/api/v2/traffic/search", "/api/v2/anomalies", "/api/service", "/api/pattern", "/api/stream/1", "/unknown", "/%2e%2e/.env", "//health/live", "/health/live/"}
	headers := []string{"", "Bearer secret", "Basic invalid", "Basic " + base64.StdEncoding.EncodeToString([]byte("audit:wrong")), "Basic " + base64.StdEncoding.EncodeToString([]byte("wrong:secret"))}
	for _, path := range paths {
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT"} {
			for _, auth := range headers {
				r := httptest.NewRequest(method, path, nil)
				r.Header.Set("Authorization", auth)
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s %s: expected uncached Basic challenge, got %d", method, path, w.Code)
				}
			}
		}
	}
}

func TestEmptyCredentialsFailClosed(t *testing.T) {
	for _, cfg := range []config.Config{{}, {Login: "audit"}, {Password: "secret"}} {
		s := &Server{Config: cfg}
		r := httptest.NewRequest("GET", "/health/live", nil)
		r.SetBasicAuth(cfg.Login, cfg.Password)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("empty credentials accepted: %d", w.Code)
		}
	}
}

func TestAuthenticatedHTTPAndOriginChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("private-ui"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Config: config.Config{Login: "audit", Password: "secret", WebDir: dir}, Metrics: &metrics.Metrics{}}
	for _, path := range []string{"/", "/health/live", "/api/v2/config", "/metrics"} {
		r := httptest.NewRequest("GET", path, nil)
		r.SetBasicAuth("audit", "secret")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("authorized %s: %d", path, w.Code)
		}
	}
	for _, tc := range []struct {
		method, origin, site string
		status               int
	}{
		{"GET", "http://evil.example", "", 403}, {"POST", "null", "", 403},
		{"POST", "", "cross-site", 403}, {"POST", "http://example.com", "same-origin", 200},
		{"POST", "", "", 200}, // CLI/farm clients do not send browser headers.
	} {
		r := httptest.NewRequest(tc.method, "http://example.com/api/v2/farm/client-credentials", nil)
		r.SetBasicAuth("audit", "secret")
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("origin case %+v: %d", tc, w.Code)
		}
	}
}

func TestWebSocketsRequireCredentialsAndSameOrigin(t *testing.T) {
	s := &Server{Config: config.Config{Login: "audit", Password: "secret"}, Hub: events.New()}
	server := httptest.NewServer(s)
	defer server.Close()
	for _, path := range []string{"/api/v2/events", "/api/v2/chat/arena"} {
		for _, tc := range []struct {
			auth   bool
			origin string
			status int
		}{{false, "", 401}, {true, "http://evil.example", 403}, {true, server.URL, 101}} {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			headers := http.Header{}
			headers.Set("Origin", tc.origin)
			if tc.auth {
				headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("audit:secret")))
			}
			c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, &websocket.DialOptions{HTTPHeader: headers})
			if c != nil {
				c.CloseNow()
			}
			cancel()
			if resp == nil || resp.StatusCode != tc.status || (tc.status == 101 && err != nil) {
				t.Fatalf("%s: expected %d, response=%v err=%v", path, tc.status, resp, err)
			}
		}
	}
}
