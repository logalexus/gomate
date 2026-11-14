package integration

import (
	"context"
	"encoding/json"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/model"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamDetailRequestContext(t *testing.T) {
	s := database(t)
	for _, c := range []*model.Chunk{
		{Token: "visible", Stream: model.Stream{Key: "visible", Service: 18080, Protocol: "TCP", Server: "192.0.2.1:18080", Client: "192.0.2.2:50000"}},
		{Token: "ignored", Stream: model.Stream{Key: "ignored", Service: 18080, Protocol: "TCP"}, Ignore: true},
	} {
		if err := s.SaveChunks(context.Background(), []*model.Chunk{c}); err != nil {
			t.Fatal(err)
		}
	}
	app := &api.Server{Store: s, Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}}
	for _, tc := range []struct {
		path   string
		auth   bool
		status int
	}{{"/api/stream/1", true, 200}, {"/api/stream/2", true, 404}, {"/api/stream/999", true, 404}, {"/api/stream/0", true, 400}, {"/api/stream/bad", true, 400}, {"/api/stream/1", false, 401}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.auth {
			r.SetBasicAuth("test", "test")
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.status == 200 {
			var stream model.Stream
			if err := json.Unmarshal(w.Body.Bytes(), &stream); err != nil {
				t.Fatal(err)
			}
			if stream.Server != "192.0.2.1:18080" || stream.Service != 18080 || stream.Protocol != "TCP" {
				t.Fatal(stream)
			}
		}
	}
}
