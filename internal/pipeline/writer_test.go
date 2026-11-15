package pipeline

import (
	"context"
	"errors"
	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"sync"
	"testing"
	"time"
)

type fakeSink struct {
	mu     sync.Mutex
	calls  int
	fail   bool
	tokens []string
	delay  time.Duration
}

func (s *fakeSink) SaveChunks(ctx context.Context, cs []*model.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	time.Sleep(s.delay)
	if s.fail {
		return errors.New("offline")
	}
	for _, c := range cs {
		s.tokens = append(s.tokens, c.Token)
	}
	return nil
}

func TestSlowCommitsDoNotDegenerateIntoTinyBatches(t *testing.T) {
	sink := &fakeSink{delay: 150 * time.Millisecond}
	m := &metrics.Metrics{}
	w := New(context.Background(), config.Config{WriteBytes: 4 << 20, DBTimeout: time.Second}, m, analyze.NewCatalog(), sink, func() {})
	for i := 0; i < 500; i++ {
		if !w.Submit(&model.Chunk{Bytes: 1, Packets: []model.Packet{{Content: []byte("a")}}}) {
			t.Fatal("unexpected overflow")
		}
	}
	w.Close()
	if sink.calls > 4 || len(sink.tokens) != 500 {
		t.Fatalf("%d commits for %d records", sink.calls, len(sink.tokens))
	}
}
func TestWriterFailureDoesNotPreventLaterProgress(t *testing.T) {
	m := &metrics.Metrics{}
	sink := &fakeSink{fail: true}
	cfg := config.Config{WriteBytes: 1 << 20, DBTimeout: 20 * time.Millisecond}
	notify := make(chan struct{}, 4)
	w := New(context.Background(), cfg, m, analyze.NewCatalog(), sink, func() { notify <- struct{}{} })
	chunk := func(token string) *model.Chunk {
		return &model.Chunk{Token: token, Bytes: 3, Packets: []model.Packet{{Content: []byte("abc")}}}
	}
	if !w.Submit(chunk("first")) {
		t.Fatal("submit rejected")
	}
	deadline := time.Now().Add(3 * time.Second)
	for m.WriteDrops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.WriteDrops.Load() != 1 {
		t.Fatal("failed batch was not accounted")
	}
	sink.mu.Lock()
	sink.fail = false
	sink.mu.Unlock()
	w.Submit(chunk("second"))
	w.Close()
	if m.PersistedChunks.Load() != 1 || m.WriteBytes.Load() != 0 {
		t.Fatal(m.Snapshot())
	}
	select {
	case <-notify:
	default:
		t.Fatal("no committed event")
	}
}

func TestQueueOverflowPersistsLossMetadata(t *testing.T) {
	sink := &fakeSink{}
	m := &metrics.Metrics{}
	w := New(context.Background(), config.Config{WriteBytes: 1, DBTimeout: time.Second}, m, analyze.NewCatalog(), sink, func() {})
	if w.Submit(&model.Chunk{Token: "lost", Stream: model.Stream{Key: "flow"}, Bytes: 3, Packets: []model.Packet{{Content: []byte("abc")}}}) {
		t.Fatal("overflow accepted")
	}
	w.Close()
	if len(sink.tokens) != 1 || sink.tokens[0] != "lost/loss" || m.WriteDrops.Load() != 1 {
		t.Fatal(sink.tokens, m.Snapshot())
	}
}

func TestRequestMetricCountsOnceWhenDatabaseRetries(t *testing.T) {
	m := &metrics.Metrics{}
	cat := analyze.NewCatalog()
	if err := cat.Replace([]model.Service{{Port: 80, HTTP: true}}, nil); err != nil {
		t.Fatal(err)
	}
	sink := &fakeSink{fail: true}
	w := New(context.Background(), config.Config{WriteBytes: 1 << 20, DBTimeout: time.Second}, m, cat, sink, func() {})
	request := []byte("GET / HTTP/1.1\r\nHost: lab\r\n\r\n")
	if !w.Submit(&model.Chunk{Token: "request", Stream: model.Stream{Key: "flow", Service: 80, Protocol: "TCP"}, Bytes: int64(len(request)), Packets: []model.Packet{{Incoming: true, Content: request}}}) {
		t.Fatal("submit failed")
	}
	w.Close()
	if sink.calls != 3 || m.Requests.Load() != 1 {
		t.Fatalf("%d retries, %d requests", sink.calls, m.Requests.Load())
	}
}
