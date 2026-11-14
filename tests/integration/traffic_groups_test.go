package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/storage"
)

func TestTrafficGroupsIncludeUnreportedSessionsAndPaginate(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	// More sessions than the old anomaly limit; only two have a saved result.
	_, err := s.Pool.Exec(ctx, `INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,ttl,client,server,first_request_method,first_request_path)
 SELECT 'group-'||i,8080,'TCP',1000+i,2000+i,0,'client','server','GET','/items?q='||i FROM generate_series(1,260) i;
 INSERT INTO gomate_anomalies(stream_id,service,score,kind,summary,reasons,dismissed,updated_ms)
 VALUES (1,8080,40,'theft','GET /items','[]',false,1000),(2,8080,90,'anomaly','GET /items','[]',true,1000);
 INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,ttl,client,server,first_request_method,first_request_path)
 SELECT 'other-'||i,8080,'TCP',5000+i,6000+i,0,'client','server','POST','/other/'||i FROM generate_series(1,105) i;
 INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,ttl,client,server)
 VALUES ('raw',9000,'TCP',9000,9000,0,'client','server');`)
	if err != nil {
		t.Fatal(err)
	}
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s}
	get := func(path string, target any) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v2/anomalies/"+path, nil)
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	var page storage.TrafficGroupPage
	get("groups?limit=100", &page)
	if page.TotalSessions != 366 || page.TotalGroups != 107 || page.TotalAnomalies != 1 || len(page.Groups) != 100 {
		t.Fatalf("bad totals: %+v", page)
	}
	g := page.Groups[0]
	if g.Path != "/items" || g.Count != 260 || g.AnomalyCount != 1 || g.DismissedCount != 1 || g.Score != 40 || g.TheftCount != 1 {
		t.Fatalf("bad group: %+v", g)
	}
	get("groups?limit=100&offset=100", &page)
	if len(page.Groups) != 7 || page.TotalSessions != 366 {
		t.Fatalf("bad second page: %+v", page)
	}
	get("groups?offset=1000", &page)
	if len(page.Groups) != 0 || page.TotalGroups != 107 {
		t.Fatalf("totals lost on empty page: %+v", page)
	}
	get("groups?service=9000", &page)
	if len(page.Groups) != 1 || page.Groups[0].Path != "" || page.TotalSessions != 1 {
		t.Fatalf("raw traffic missing: %+v", page)
	}
	get("groups?service=8080&sort=time&limit=1", &page)
	if page.Groups[0].Path != "/other/105" {
		t.Fatal("time sort", page)
	}
	get("groups?period=1", &page)
	if page.TotalSessions != 0 || len(page.Groups) != 0 {
		t.Fatal("period ignored", page)
	}
	base := "sessions?service=8080&protocol=TCP&method=GET&path=" + url.QueryEscape("/items") + "&limit=50"
	var members storage.TrafficSessionPage
	seen := map[int64]bool{}
	var before int64
	for {
		get(fmt.Sprintf("%s&before=%d", base, before), &members)
		for _, v := range members.Items {
			if seen[v.StreamID] {
				t.Fatal("duplicate cursor row", v.StreamID)
			}
			seen[v.StreamID] = true
			if v.StreamID > 2 && (v.HasAnomaly || v.Score != 0 || len(v.Reasons) != 0) {
				t.Fatal("normal misrepresented", v)
			}
			if v.Request != fmt.Sprintf("GET /items?q=%d", v.StreamID) {
				t.Fatal("query lost", v)
			}
		}
		before = members.NextBefore
		if before == 0 {
			break
		}
	}
	if len(seen) != 260 {
		t.Fatal("missing sessions", len(seen))
	}
	get(base+"&alerts=1", &members)
	if len(members.Items) != 1 || members.Items[0].StreamID != 1 || !members.Items[0].HasAnomaly {
		t.Fatal("alert filter", members)
	}
	// Payload-only cleanup keeps membership; removing the session cascades its alert.
	_, err = s.Pool.Exec(ctx, "UPDATE gomate_streams SET payload_purged=true WHERE id=260; DELETE FROM gomate_streams WHERE id=1")
	if err != nil {
		t.Fatal(err)
	}
	get(base, &members)
	if !members.Items[0].PayloadPurged {
		t.Fatal("payload retention marker lost")
	}
	get("groups?service=8080&limit=200", &page)
	for _, v := range page.Groups {
		if v.Path == "/items" && (v.Count != 259 || v.AnomalyCount != 0 || v.Score != 0) {
			t.Fatal("deleted session still counted", v)
		}
	}
	for _, path := range []string{"groups?service=-1", "groups?limit=0", "groups?limit=201", "groups?period=9223372036854775807", "groups?offset=-1", "sessions?service=0&protocol=TCP", "sessions?service=80", "sessions?service=80&protocol=TCP&before=-1"} {
		r := httptest.NewRequest("GET", "/api/v2/anomalies/"+path, nil)
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("invalid query accepted", path, w.Code)
		}
	}
}
