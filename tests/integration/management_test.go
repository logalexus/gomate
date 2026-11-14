package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"gomate/internal/analyze"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"gomate/internal/storage"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettingsRevisionRetentionAndManualCleanup(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	v, e := s.InitSettings(ctx, model.RuntimeSettings{CaptureEnabled: false, AutoIPs: true, CleanupIntervalSeconds: 5, PreserveFavorites: true})
	if e != nil {
		t.Fatal(e)
	}
	stale := v
	v.PacketRetentionSeconds = 60
	v.SessionRetentionSeconds = 120
	if e = s.PutSettings(ctx, &v); e != nil {
		t.Fatal(e)
	}
	if e = s.PutSettings(ctx, &stale); e != storage.ErrRevision {
		t.Fatal(e)
	}
	saved, e := s.InitSettings(ctx, model.RuntimeSettings{Interface: "should-not-override"})
	if e != nil || saved.Revision != 2 || saved.PacketRetentionSeconds != 60 {
		t.Fatal(saved, e)
	}
	now := time.Now()
	for i := 0; i < 3; i++ {
		stamp := now.Add(-90 * time.Second).UnixMilli()
		if i == 2 {
			stamp = now.UnixMilli()
		}
		c := &model.Chunk{Token: fmt.Sprint(i), Stream: model.Stream{Key: fmt.Sprint(i), Service: 80, Protocol: "TCP", Start: stamp, End: stamp}, Bytes: 3, Packets: []model.Packet{{Timestamp: stamp, Content: []byte("abc")}}}
		if e = s.SaveChunks(ctx, []*model.Chunk{c}); e != nil {
			t.Fatal(e)
		}
	}
	rows, _ := s.Streams(ctx, 80, model.Page{PageSize: 10})
	favorite := rows[2].ID
	s.Favorite(ctx, favorite, true)
	if e = s.Retain(ctx, v); e != nil {
		t.Fatal(e)
	}
	packets, _ := s.Packets(ctx, rows[1].ID, model.Page{PageSize: 10})
	if len(packets) != 0 {
		t.Fatal("expired content retained")
	}
	packets, _ = s.Packets(ctx, favorite, model.Page{PageSize: 10})
	if len(packets) != 1 {
		t.Fatal("favorite content removed")
	}
	id, e := s.StartCleanup(ctx, "sessions", true, 80)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StartCleanup(ctx, "packets", true, 0); e == nil {
		t.Fatal("concurrent cleanup accepted")
	}
	for i := 0; i < 5; i++ {
		if _, e = s.StepCleanup(ctx); e != nil {
			t.Fatal(e)
		}
	}
	jobs, e := s.CleanupJobs(ctx)
	if e != nil || jobs[0].ID != id || jobs[0].Status != "completed" {
		t.Fatal(jobs, e)
	}
	rows, e = s.Streams(ctx, 80, model.Page{PageSize: 10})
	if e != nil || len(rows) != 1 || rows[0].ID != favorite {
		t.Fatal(rows, e)
	}
}
func TestCleanupSnapshotKeepsNewTraffic(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	c := &model.Chunk{Token: "old", Stream: model.Stream{Key: "active", Service: 80, Protocol: "TCP"}, Packets: []model.Packet{{Content: []byte("old")}}}
	s.SaveChunks(ctx, []*model.Chunk{c})
	s.StartCleanup(ctx, "sessions", true, 80)
	c.Token = "new"
	c.Packets = []model.Packet{{Content: []byte("new")}}
	s.SaveChunks(ctx, []*model.Chunk{c})
	for i := 0; i < 4; i++ {
		if _, e := s.StepCleanup(ctx); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.Streams(ctx, 80, model.Page{PageSize: 10})
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	ps, e := s.Packets(ctx, rows[0].ID, model.Page{PageSize: 10})
	if e != nil || len(ps) != 1 || string(ps[0].Content) != "new" || !rows[0].PayloadPurged {
		t.Fatal(ps, rows, e)
	}
}
func TestPatternManagementAPIs(t *testing.T) {
	s := database(t)
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s, Catalog: analyze.NewCatalog(), Metrics: &metrics.Metrics{}, Hub: events.New()}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	p := model.Pattern{Name: "Flag", Group: "Flags", Value: "FLAG_", Color: "#112233", SearchType: "SUBSTRING", Direction: "OUTPUT", Action: "FIND", Enabled: false}
	b, _ := json.Marshal(p)
	w := request("POST", "/api/pattern/", string(b))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.Enabled {
		t.Fatal("disabled creation ignored")
	}
	w = request("POST", "/api/v2/patterns-bulk", fmt.Sprintf(`{"ids":[%d],"action":"enable"}`, p.ID))
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	b, _ = json.Marshal(map[string]any{"pattern": p, "text": "abc FLAG_xyz", "incoming": false})
	w = request("POST", "/api/v2/pattern-test", string(b))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"startPosition":4`) {
		t.Fatal(w.Code, w.Body.String())
	}
	old := p.ID
	p.Value = "NEW_FLAG"
	p.Enabled = true
	b, _ = json.Marshal(p)
	w = request("PUT", fmt.Sprintf("/api/pattern/%d", old), string(b))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID == old {
		t.Fatal("semantic edit reused historical pattern identity")
	}
	all, _ := s.Patterns(context.Background())
	if !all[0].Deleted || all[1].Group != "Flags" {
		t.Fatal(all)
	}
	w = request("POST", "/api/v2/cleanup", `{"mode":"packets"}`)
	if w.Code != 400 {
		t.Fatal("deletion without confirmation", w.Code)
	}
}
