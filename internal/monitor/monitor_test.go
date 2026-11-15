package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gomate/internal/dockerui"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureProc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "net"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"stat":    "cpu  100 20 30 400 10 4 6 2 80 10\ncpu0 1\ncpu1 1\n",
		"meminfo": "MemTotal: 10000 kB\nMemAvailable: 4000 kB\n",
		"loadavg": "1.25 2 3 1/200 99\n", "uptime": "1000.50 400\n",
		"net/dev": "eth0: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0\nlo: 99999 0 0 0 0 0 0 0 99999 0 0 0 0 0 0 0\nvethabc: 99999 0 0 0 0 0 0 0 99999 0 0 0 0 0 0 0\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func TestHostCountersAndMissingMounts(t *testing.T) {
	proc := fixtureProc(t)
	h, c, err := readHost(proc, t.TempDir(), time.Now())
	if err != nil || c.total != 572 || c.idle != 410 || c.rx != 1000 || c.tx != 2000 || h.CPUs != 2 || *h.MemoryUsed != 6000*1024 || h.Load1 != 1.25 || h.DiskTotal == nil {
		t.Fatalf("host=%+v counters=%+v err=%v", h, c, err)
	}
	h, _, err = readHost(proc, "/nonexistent-gomate-host", time.Now())
	if err != nil || h.DiskUsed != nil || h.DiskError == "" {
		t.Fatal("missing root must be unavailable", h, err)
	}
	if _, _, err = readHost("/nonexistent-gomate-proc", "/", time.Now()); err == nil {
		t.Fatal("must not fall back to container proc")
	}
	if err = os.Remove(filepath.Join(proc, "net/dev")); err != nil {
		t.Fatal(err)
	}
	h, _, err = readHost(proc, "/", time.Now())
	if err != nil || h.NetworkError == "" || h.RX != nil {
		t.Fatal("missing network must not be zero", h, err)
	}
}
func sample(t *testing.T, at time.Time, total, system, rx uint64) dockerui.Stats {
	t.Helper()
	var s dockerui.Stats
	err := json.Unmarshal([]byte(fmt.Sprintf(`{"read":%q,"cpu_stats":{"cpu_usage":{"total_usage":%d},"system_cpu_usage":%d,"online_cpus":8},"memory_stats":{"usage":1000,"limit":8000,"stats":{"inactive_file":200}},"networks":{"eth0":{"rx_bytes":%d,"tx_bytes":0}},"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":%d}]}}`, at.Format(time.RFC3339Nano), total, system, rx, rx)), &s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestContainerCalculationsAndResets(t *testing.T) {
	now := time.Now()
	prev := sample(t, now.Add(-5*time.Second), 100, 1000, 1000)
	s := sample(t, now, 200, 1800, 6000)
	p := containerPoint(s, previous{prev, now.Add(-6 * time.Second)}, now)
	if p.CPU == nil || *p.CPU != 100 || *p.MemoryUsed != 800 || *p.RX != 1000 || *p.Read != 1000 || *p.TX != 0 {
		t.Fatalf("bad rates: %+v", p)
	}
	first := containerPoint(s, previous{}, now)
	if first.CPU != nil || first.RX != nil || first.MemoryUsed == nil {
		t.Fatal("first sample has no rate")
	}
	s.Memory.Stats["total_inactive_file"] = 300
	if *containerPoint(s, previous{}, now).MemoryUsed != 700 {
		t.Fatal("cgroup v1 cache precedence")
	}
	s.CPU.Usage.Total = 1
	p = containerPoint(s, previous{prev, now.Add(-5 * time.Second)}, now)
	if p.CPU != nil || p.RX != nil || p.Read != nil {
		t.Fatal("restart must create a gap")
	}
	s.CPU.Usage.Total = 200
	s.Read = prev.Read
	if containerPoint(s, previous{prev, now}, now).CPU != nil {
		t.Fatal("cached daemon sample must create a gap")
	}
	s.Read = now
	s.Networks = nil
	s.Block.Entries = nil
	p = containerPoint(s, previous{prev, now.Add(-5 * time.Second)}, now)
	if p.RX != nil || p.Read != nil {
		t.Fatal("unavailable counters must not show zero")
	}
	if rate(1, 2, 5) != nil || rate(3, 2, 0) != nil {
		t.Fatal("counter resets and zero intervals")
	}
}
func TestRingBoundedChronologicalAndCutoff(t *testing.T) {
	var r ring
	for i := int64(0); i < 2000; i++ {
		r.add(Point{Time: i})
	}
	all := r.since(0)
	if len(all) != 720 || all[0].Time != 1280 || all[719].Time != 1999 || len(r.since(1990)) != 10 {
		t.Fatal("bad bounded history")
	}
	all[0].Time = 0
	if r.since(0)[0].Time != 1280 {
		t.Fatal("snapshot aliases ring")
	}
}

type fakeSource struct {
	list                []dockerui.Container
	fail                bool
	calls, active, peak atomic.Int32
}

func (f *fakeSource) List(context.Context) ([]dockerui.Container, error) {
	if f.fail {
		return nil, errors.New("offline")
	}
	return append([]dockerui.Container(nil), f.list...), nil
}
func (f *fakeSource) Stats(ctx context.Context, id string) (dockerui.Stats, error) {
	f.calls.Add(1)
	active := f.active.Add(1)
	defer f.active.Add(-1)
	for old := f.peak.Load(); active > old; old = f.peak.Load() {
		if f.peak.CompareAndSwap(old, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return dockerui.Stats{}, ctx.Err()
	case <-time.After(time.Millisecond):
	}
	s := dockerui.Stats{}
	s.Memory.Usage = 123
	s.Memory.Limit = 1024
	return s, nil
}
func TestSharedCollectorBoundsGapsAndPruning(t *testing.T) {
	f := &fakeSource{}
	for i := 0; i < 130; i++ {
		f.list = append(f.list, dockerui.Container{ID: fmt.Sprintf("%064x", i), Name: fmt.Sprintf("service-%03d", i), State: "running"})
	}
	f.list[0].State = "exited"
	m := New(f, fixtureProc(t), t.TempDir())
	m.collect(context.Background())
	got := m.snapshot(60, f.list[1].ID)
	if !got.Truncated || len(got.Containers) != 128 || len(m.histories) != 128 || f.calls.Load() != 127 || f.peak.Load() > 4 || f.peak.Load() < 2 || len(got.ContainerHistory) != 1 {
		t.Fatal("collection bounds", len(got.Containers), f.calls.Load(), f.peak.Load())
	}
	// Concurrent readers use the shared snapshot while collection is running.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.collect(context.Background()) }()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				m.snapshot(60, f.list[1].ID)
			}
		}()
	}
	wg.Wait()
	if f.calls.Load() != 254 {
		t.Fatal("readers must not query Docker")
	}
	f.fail = true
	m.collect(context.Background())
	got = m.snapshot(60, f.list[1].ID)
	if got.Error == "" || len(got.Containers) != 128 || got.Containers[1].Point.MemoryUsed != nil || len(got.ContainerHistory) != 3 || got.ContainerHistory[2].CPU != nil {
		t.Fatal("outage must retain identities with gaps")
	}
	f.fail = false
	f.list = f.list[:1]
	m.collect(context.Background())
	if len(m.histories) != 1 || len(m.prev) != 0 {
		t.Fatal("removed/stopped containers must release history and baselines")
	}
}
func TestMonitorValidationAndAuthentication(t *testing.T) {
	m := New(&fakeSource{}, fixtureProc(t), t.TempDir())
	m.collect(context.Background())
	h := dockerui.Gateway(nil, "a-long-test-monitor-token", m)
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{"GET", "/monitor", "", 401}, {"GET", "/monitor", "Bearer a-long-test-monitor-token", 200},
		{"POST", "/monitor", "Bearer a-long-test-monitor-token", 405},
		{"GET", "/monitor?minutes=10000", "Bearer a-long-test-monitor-token", 400},
		{"GET", "/monitor?container=../../etc/passwd", "Bearer a-long-test-monitor-token", 400},
		{"GET", "/monitor?minutes=60&container=" + strings.Repeat("a", 64), "Bearer a-long-test-monitor-token", 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s = %d", tc.method, tc.path, w.Code)
		}
	}
}

type stalledSource struct{ entered chan struct{} }

func (f *stalledSource) List(ctx context.Context) ([]dockerui.Container, error) {
	close(f.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*stalledSource) Stats(context.Context, string) (dockerui.Stats, error) {
	panic("stats must not be called when listing fails")
}
func TestStalledDockerDoesNotBlockReadersAndCollectionTimesOut(t *testing.T) {
	f := &stalledSource{entered: make(chan struct{})}
	m := New(f, fixtureProc(t), t.TempDir())
	done := make(chan struct{})
	go func() { m.collect(context.Background()); close(done) }()
	<-f.entered
	read := make(chan struct{})
	go func() { m.snapshot(60, ""); close(read) }()
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("snapshot blocked by stalled Docker")
	}
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("collection exceeded its 4-second timeout")
	}
	if m.snapshot(60, "").Error == "" {
		t.Fatal("timed out collection must report unavailable Docker")
	}
}
