package api

import (
	"encoding/json"
	"gomate/internal/config"
	"net/http/httptest"
	"testing"
)

func TestFarmClientCredentialsRequireAuthenticationAndExplicitRequest(t *testing.T) {
	s := &Server{Config: config.Config{Login: "test", Password: "test'password"}}
	for _, tc := range []struct {
		method, origin string
		auth           bool
		status         int
	}{
		{"POST", "", false, 401}, {"GET", "", true, 405}, {"POST", "https://other.example", true, 403}, {"POST", "", true, 200},
	} {
		r := httptest.NewRequest(tc.method, "http://gomate/api/v2/farm/client-credentials", nil)
		if tc.auth {
			r.SetBasicAuth(s.Config.Login, s.Config.Password)
		}
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.method, w.Code)
		}
		if tc.status == 200 {
			var got map[string]string
			if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
				t.Fatal(e)
			}
			if got["username"] != s.Config.Login || got["password"] != s.Config.Password {
				t.Fatal("credentials mismatch")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("credentials cacheable")
			}
		}
	}
}
