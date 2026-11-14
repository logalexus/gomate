package control

import (
	"context"
	"errors"
	"gomate/internal/analyze"
	"gomate/internal/config"
	"gomate/internal/flow"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSource struct{ closed atomic.Bool }

func (s *fakeSource) Run(ctx context.Context, _ *flow.Engine, m *metrics.Metrics) error {
	m.CaptureRunning.Store(true)
	defer m.CaptureRunning.Store(false)
	<-ctx.Done()
	return ctx.Err()
}
func (s *fakeSource) Close() { s.closed.Store(true) }
func TestDynamicCapturePreparationFailureRetentionAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{Shards: 1, QueuePackets: 8, MaxFlows: 16, MaxFlowBytes: 4096, FlushInterval: time.Second, IdleTimeout: time.Second}
	m := New(ctx, cfg, analyze.NewCatalog(), &metrics.Metrics{}, func(*model.Chunk) bool { return true }, func() {})
	var sources []*fakeSource
	m.resolve = func(model.RuntimeSettings) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
	}
	m.prepare = func(name string) (Source, error) {
		if name == "missing" {
			return nil, errors.New("cannot open")
		}
		s := &fakeSource{}
		sources = append(sources, s)
		return s, nil
	}
	defer m.Close()
	v := model.RuntimeSettings{Revision: 1, CaptureEnabled: true, Interface: "first", AutoIPs: true, CleanupIntervalSeconds: 5, PreserveFavorites: true}
	persist := func(_ context.Context, v *model.RuntimeSettings) error { v.Revision++; return nil }
	if e := m.Apply(ctx, v, persist); e != nil {
		t.Fatal(e)
	}
	waitRunning := func() {
		t.Helper()
		until := time.Now().Add(time.Second)
		for !m.Status().Running && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if !m.Status().Running {
			t.Fatal(m.Status())
		}
	}
	waitRunning()
	v = m.Settings()
	v.Interface = "missing"
	if e := m.Apply(ctx, v, persist); e == nil {
		t.Fatal("bad device accepted")
	}
	if !m.Status().Running || m.Settings().Interface != "first" || sources[0].closed.Load() {
		t.Fatal("existing source stopped on failed preparation")
	}
	v = m.Settings()
	v.PacketRetentionSeconds = 60
	if e := m.Apply(ctx, v, persist); e != nil {
		t.Fatal(e)
	}
	if len(sources) != 1 {
		t.Fatal("retention change restarted capture")
	}
	v = m.Settings()
	v.Interface = "next"
	if e := m.Apply(ctx, v, func(context.Context, *model.RuntimeSettings) error { return errors.New("DB down") }); e == nil {
		t.Fatal("save failure ignored")
	}
	if sources[0].closed.Load() || !sources[1].closed.Load() {
		t.Fatal("incorrect rollback")
	}
	if e := m.Apply(ctx, v, persist); e != nil {
		t.Fatal(e)
	}
	waitRunning()
	if !sources[0].closed.Load() || m.Status().Interface != "next" {
		t.Fatal(m.Status())
	}
	v = m.Settings()
	v.CaptureEnabled = false
	if e := m.Apply(ctx, v, persist); e != nil {
		t.Fatal(e)
	}
	if m.Status().Running || !sources[2].closed.Load() {
		t.Fatal("stop failed")
	}
}
