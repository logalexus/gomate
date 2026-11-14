package integration

import (
	"context"
	"gomate/internal/model"
	"testing"
	"time"
)

func TestLookbackCheckpointResumeCancelAndMetadata(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	var packets []model.Packet
	for i := 0; i < 7; i++ {
		packets = append(packets, model.Packet{Content: []byte("padding"), Incoming: true})
	}
	packets = append(packets, model.Packet{Content: []byte("FL"), Incoming: true}, model.Packet{Content: []byte("AG"), Incoming: true}, model.Packet{Content: []byte("decoded"), Kind: "grpc", Metadata: map[string]string{"path": "/Echo/Run", "streamId": "1"}, Ungzipped: true})
	ua := "testhash"
	c := &model.Chunk{Token: "lb", Stream: model.Stream{Key: "lb", Protocol: "TCP", Service: 80, Start: now, End: now, UserAgentHash: &ua}, Packets: packets, Bytes: 100}
	if e := s.SaveChunks(ctx, []*model.Chunk{c}); e != nil {
		t.Fatal(e)
	}
	p := model.Pattern{ID: 3, Name: "flag", Value: "FLAG", Enabled: true, Color: "#112233", SearchType: "SUBSTRING", Direction: "BOTH", Action: "FIND"}
	id, e := s.StartLookback(ctx, p, time.Now().Add(-time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StartLookback(ctx, p, time.Now()); e == nil {
		t.Fatal("second active job accepted")
	}
	if _, e = s.StepLookback(ctx); e != nil {
		t.Fatal(e)
	}
	jobs, e := s.Jobs(ctx)
	if e != nil || jobs[0].Processed != 8 {
		t.Fatal(jobs, e)
	}
	// No process-local state is retained: every Step loads its cursor and tails.
	for i := 0; i < 4; i++ {
		if _, e = s.StepLookback(ctx); e != nil {
			t.Fatal(e)
		}
	}
	jobs, e = s.Jobs(ctx)
	if e != nil || jobs[0].Status != "completed" || jobs[0].Matched != 1 {
		t.Fatal(jobs, e)
	}
	streams, e := s.Streams(ctx, 80, model.Page{PageSize: 10})
	if e != nil || len(streams) != 1 || len(streams[0].Patterns) != 1 || streams[0].UserAgentHash == nil {
		t.Fatal(streams, e)
	}
	stored, e := s.Packets(ctx, streams[0].ID, model.Page{PageSize: 20})
	if e != nil || len(stored[8].Matches) != 1 || !stored[8].Matches[0].CrossBoundary || stored[9].Kind != "grpc" || !stored[9].Ungzipped || stored[9].Metadata["path"] != "/Echo/Run" {
		t.Fatal(stored, e)
	}
	id, e = s.StartLookback(ctx, p, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CancelLookback(ctx, id); e != nil {
		t.Fatal(e)
	}
	changed, e := s.StepLookback(ctx)
	if e != nil || changed {
		t.Fatal(changed, e)
	}
}

func TestEmptyDecodedMessageAndRepresentationPagination(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	c := &model.Chunk{Token: "empty", Stream: model.Stream{Key: "empty", Protocol: "TCP", Service: 80}, Packets: []model.Packet{{Kind: "grpc", Metadata: map[string]string{"path": "/Empty"}}, {Content: []byte("raw")}, {Kind: "http2-body", Content: []byte("body")}}}
	if e := s.SaveChunks(ctx, []*model.Chunk{c}); e != nil {
		t.Fatal(e)
	}
	streams, e := s.Streams(ctx, 0, model.Page{PageSize: 10})
	if e != nil {
		t.Fatal(e)
	}
	out, more, e := s.PacketsPage(ctx, streams[0].ID, model.Page{PageSize: 1, Representation: "decoded"})
	if e != nil || len(out) != 1 || !more || out[0].Kind != "grpc" || out[0].Content == nil {
		t.Fatal(out, more, e)
	}
	cursor := out[0].ID
	out, more, e = s.PacketsPage(ctx, streams[0].ID, model.Page{StartingFrom: &cursor, PageSize: 1, Representation: "decoded"})
	if e != nil || len(out) != 1 || more || out[0].Kind != "http2-body" {
		t.Fatal(out, more, e)
	}
}
