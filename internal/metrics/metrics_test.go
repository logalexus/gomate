package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRequestRateWindowAndIdleDecay(t *testing.T) {
	var w requestWindow
	now := time.Unix(1000, 0)
	w.record(20, now)
	w.record(30, now.Add(2*time.Second))
	for _, tc := range []struct {
		after time.Duration
		want  float64
	}{
		{2 * time.Second, 10}, {5 * time.Second, 6}, {7 * time.Second, 0},
	} {
		if got := w.rate(now.Add(tc.after)); got != tc.want {
			t.Errorf("at %s got %g, want %g", tc.after, got, tc.want)
		}
	}
	w.record(5, now.Add(time.Hour))
	if got := w.rate(now.Add(time.Hour)); got != 1 {
		t.Fatalf("stale bucket reused: %g", got)
	}
}
func TestConcurrentRequestsAndExport(t *testing.T) {
	var m Metrics
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				m.RecordRequests(5)
				m.RequestRate()
				m.Snapshot()
			}
		})
	}
	wg.Wait()
	if got := m.Snapshot()["requests_total"]; got != 5000 {
		t.Fatalf("lost requests: %d", got)
	}
	m.RecordRequests(0)
	m.RecordRequests(-1)
	if got := m.Requests.Load(); got != 5000 {
		t.Fatalf("invalid recording changed counter: %d", got)
	}
	var b bytes.Buffer
	m.Write(&b)
	if !strings.Contains(b.String(), "gomate_requests_per_second ") || !strings.Contains(b.String(), "gomate_requests_total 5000\n") {
		t.Fatal(b.String())
	}
}
