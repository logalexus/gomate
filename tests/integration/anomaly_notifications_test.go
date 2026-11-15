package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gomate/internal/anomaly"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/events"
)

func TestAnomalyNotificationVisibility(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	_, err := s.Pool.Exec(ctx, `INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,ttl,client,server,pattern_ids)
 SELECT 'notice-'||i,80,'TCP',i,i,0,'client','server','{}' FROM generate_series(1,4) i`)
	if err != nil {
		t.Fatal(err)
	}
	hub := events.New()
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s, Hub: hub}
	read := func(cursor string) events.AnomalyNotifications {
		r := httptest.NewRequest("GET", "/api/v2/anomalies/notifications?cursor="+url.QueryEscape(cursor), nil)
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		var feed events.AnomalyNotifications
		if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil {
			t.Fatal(err)
		}
		return feed
	}
	start := read("")
	rows := []anomaly.Result{}
	for i := int64(1); i <= 5; i++ {
		rows = append(rows, anomaly.Result{StreamID: i, Service: 80, Score: 50, Reasons: []anomaly.Reason{}})
	}
	if err := s.SaveAnomalies(ctx, rows); err != nil {
		t.Fatal(err)
	}
	hub.PublishAnomalies(rows)
	_, err = s.Pool.Exec(ctx, `UPDATE gomate_anomalies SET dismissed=true WHERE stream_id=2;
 UPDATE gomate_streams SET ignored=true WHERE id=3; DELETE FROM gomate_streams WHERE id=4`)
	if err != nil {
		t.Fatal(err)
	}
	feed := read(start.Cursor)
	if len(feed.Items) != 1 || feed.Items[0].StreamID != 1 {
		t.Fatalf("invisible sessions notified: %+v", feed)
	}
	if len(read(feed.Cursor).Items) != 0 {
		t.Fatal("cursor not advanced past hidden sessions")
	}
}
