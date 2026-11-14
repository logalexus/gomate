package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/control"
	"gomate/internal/metrics"
	"gomate/internal/model"
)

func TestCommonTimeoutAllowsSlowGatewayReads(t *testing.T) {
	for _, path := range []string{"/api/v2/containers", "/api/v2/runner", "/api/v2/docker-services"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Gomate-Timeout-Seconds") != "30" {
					t.Error("missing common gateway budget")
				}
				select {
				case <-time.After(6 * time.Second):
					w.Write([]byte(`[]`))
				case <-r.Context().Done():
				}
			}))
			defer gateway.Close()
			cfg := config.Config{Login: "test", Password: "test", DiscoveryURL: gateway.URL}
			s := &Server{Config: cfg, Control: control.New(context.Background(), cfg, analyze.NewCatalog(), &metrics.Metrics{}, nil, func() {})}
			if err := s.Control.Apply(context.Background(), model.RuntimeSettings{RequestTimeoutSeconds: 30, CleanupIntervalSeconds: 5}, nil); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", path, nil)
			r.SetBasicAuth("test", "test")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
