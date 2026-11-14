package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"gomate/internal/api"
	"gomate/internal/config"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gomate/internal/model"
	"gomate/internal/storage"
)

func TestBehaviorFingerprintsSurviveRepetitionRestartAndPayloadCleanup(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	var chunks []*model.Chunk
	// Same endpoint and structure, fresh credentials/nonces every time, two stable
	// command values. No anomaly rows at all: this models a detector used to attacks.
	for i := 0; i < 220; i++ {
		command := "list"
		if i < 120 {
			command = "cat /srv/flag; id"
		}
		body := fmt.Sprintf(`{"command":%q,"nonce":%q}`, command, fmt.Sprintf("nonce-%d", i))
		raw := fmt.Sprintf("POST /exec HTTP/1.1\r\nContent-Type: application/json\r\nCookie: session=s-%d\r\n\r\n%s", i, body)
		chunks = append(chunks, &model.Chunk{Token: fmt.Sprint(i), Stream: model.Stream{Key: fmt.Sprint(i), Service: 8080, Protocol: "TCP", Start: int64(1000 + i), End: int64(2000 + i)}, Decoded: true, Packets: []model.Packet{{Incoming: true, Content: []byte(raw)}, {Incoming: true, Kind: "http", Metadata: map[string]string{"method": "POST", "path": "/exec"}, Content: []byte(raw)}}})
	}
	if err := s.SaveChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}
	// Receipt retries must not double descriptors or change the fingerprint.
	if err := s.SaveChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}
	q := storage.TrafficGroupQuery{Limit: 100}
	page, err := s.BehaviorGroups(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalSessions != 220 || page.TotalGroups != 2 || page.TotalAnomalies != 0 {
		t.Fatalf("repeated exploit merged or vanished: %+v", page)
	}
	var attack storage.TrafficGroup
	for _, g := range page.Groups {
		if strings.Contains(strings.Join(g.Fields, " "), "cat /srv/flag") {
			attack = g
		}
	}
	if attack.Count != 120 || len(attack.VariableFields) != 2 {
		t.Fatalf("tokens fragmented exploit: %+v", attack)
	}
	members := storage.TrafficGroupQuery{Service: 8080, Protocol: "TCP", Method: "POST", Path: "/exec", Fingerprint: attack.Fingerprint, Limit: 50}
	n := 0
	seen := map[int64]bool{}
	for {
		p, e := s.TrafficGroupSessions(ctx, members)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range p.Items {
			if seen[v.StreamID] || v.HasAnomaly {
				t.Fatal("bad membership", v)
			}
			seen[v.StreamID] = true
			n++
		}
		if p.NextBefore == 0 {
			break
		}
		members.Before = p.NextBefore
	}
	if n != 120 {
		t.Fatal("incomplete pagination", n)
	}
	// Time filter must not relearn variability from only the returned rows.
	q.SinceMs = 1100
	p, err := s.BehaviorGroups(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range p.Groups {
		if g.Count == 20 && g.Fingerprint != attack.Fingerprint {
			t.Fatal("period changed fingerprint")
		}
	}
	q.SinceMs = 0
	// Upgrade backfill must reproduce the same fingerprints, excluding original
	// transport copies that the protocol decoder also retains.
	if _, err = s.Pool.Exec(ctx, `UPDATE gomate_streams SET behavior=NULL`); err != nil {
		t.Fatal(err)
	}
	for {
		changed, e := s.StepBehaviorBackfill(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !changed {
			break
		}
	}
	p, err = s.BehaviorGroups(ctx, q)
	if err != nil || p.TotalGroups != 2 {
		t.Fatal("backfill changed groups", p, err)
	}
	found := false
	for _, g := range p.Groups {
		found = found || g.Fingerprint == attack.Fingerprint
	}
	if !found {
		t.Fatal("backfill changed fingerprint")
	}
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Store: s}
	get := func(path string, target any) int {
		r := httptest.NewRequest("GET", "/api/v2/anomalies/"+path, nil)
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code == 200 {
			if e := json.Unmarshal(w.Body.Bytes(), target); e != nil {
				t.Fatal(e)
			}
		}
		return w.Code
	}
	if code := get("groups?grouping=behavior", &p); code != 200 || p.TotalGroups != 2 {
		t.Fatal("behavior API", code, p)
	}
	var memberPage storage.TrafficSessionPage
	if code := get("sessions?service=8080&protocol=TCP&method=POST&path="+url.QueryEscape("/exec")+"&fingerprint="+attack.Fingerprint, &memberPage); code != 200 || len(memberPage.Items) != 100 {
		t.Fatal("fingerprint membership API", code, len(memberPage.Items))
	}
	if code := get("sessions?service=8080&protocol=TCP&fingerprint=invalid", &memberPage); code != 400 {
		t.Fatal("invalid fingerprint accepted", code)
	}
	// A fresh Store has no detector state. Persisted features still identify attacks.
	reopened, err := storage.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	p, err = reopened.BehaviorGroups(ctx, q)
	if err != nil || p.TotalGroups != 2 {
		t.Fatal("restart lost fingerprints", p, err)
	}
	if _, err = s.Pool.Exec(ctx, `DELETE FROM gomate_packets; UPDATE gomate_streams SET payload_purged=true`); err != nil {
		t.Fatal(err)
	}
	p, err = s.BehaviorGroups(ctx, q)
	if err != nil || p.TotalGroups != 2 || p.TotalSessions != 220 {
		t.Fatal("payload cleanup lost fingerprints", p, err)
	}
	// Deleting sessions removes their descriptors as well; continuing traffic stays.
	if _, err = s.Pool.Exec(ctx, `DELETE FROM gomate_streams WHERE id<=10`); err != nil {
		t.Fatal(err)
	}
	p, err = s.BehaviorGroups(ctx, q)
	if err != nil || p.TotalGroups != 2 || p.TotalSessions != 210 {
		t.Fatal("retention membership", p, err)
	}
	for _, g := range p.Groups {
		if g.Count == 110 && g.Fingerprint != attack.Fingerprint {
			t.Fatal("repetition trained away exploit")
		}
	}
}

func TestBehaviorIncludesSubsequentClientMessages(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		stream := model.Stream{Key: fmt.Sprint(i), Service: 9000, Protocol: "TCP", Start: 1000, End: 2000}
		for j := 0; j < 2; j++ {
			body := `{"op":"hello"}`
			if j == 1 {
				if i < 10 {
					body = `{"op":"exploit"}`
				} else {
					body = `{"op":"read"}`
				}
			}
			c := &model.Chunk{Token: fmt.Sprintf("%d/%d", i, j), Stream: stream, Packets: []model.Packet{{Incoming: true, Kind: "websocket", Content: []byte(body)}}}
			if err := s.SaveChunks(ctx, []*model.Chunk{c}); err != nil {
				t.Fatal(err)
			}
		}
	}
	p, e := s.BehaviorGroups(ctx, storage.TrafficGroupQuery{Limit: 100})
	if e != nil || p.TotalGroups != 2 || p.TotalSessions != 20 {
		t.Fatal("only first message compared", p, e)
	}
}
