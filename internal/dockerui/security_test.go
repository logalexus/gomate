package dockerui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayAllRoutesRequireStrongToken(t *testing.T) {
	for _, token := range []string{"", "short", "replace-with-another-random-secret-at-least-24-characters", strings.Repeat("k", 32)} {
		handler := Gateway(nil, token, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthorized request reached handler") }))
		for _, path := range []string{"/services", "/monitor", "/containers", "/containers/id/start", "/containers/id/logs", "/runner", "/runner/jobs", "/unknown", "//services"} {
			auths := []string{"", "Basic dGVzdDp0ZXN0", "Bearer incorrect"}
			if len(token) < 24 || strings.HasPrefix(token, "replace-") {
				auths = append(auths, "Bearer "+token)
			}
			for _, auth := range auths {
				for _, method := range []string{"GET", "POST", "DELETE", "OPTIONS"} {
					r := httptest.NewRequest(method, path, nil)
					r.Header.Set("Authorization", auth)
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, r)
					if w.Code != 401 {
						t.Fatalf("%s %s: %d", method, path, w.Code)
					}
				}
			}
		}
	}
}
