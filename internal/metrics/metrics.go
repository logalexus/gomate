package metrics

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	Requests                                                                                         atomic.Int64
	requestRate                                                                                      requestWindow
	AnalysisBytes, AnalysisEvictions, DecodeErrors                                                   atomic.Int64
	Captured, Accepted, Filtered, Unsupported, CaptureDrops, QueueDrops, FlowDrops, WriteDrops, Gaps atomic.Int64
	ActiveBytes, QueueBytes, WriteBytes, ActiveFlows, PersistedChunks, PersistedBytes, DBErrors      atomic.Int64
	LastCapture, LastProcess, LastCommit, PendingSince                                               atomic.Int64
	CaptureRunning                                                                                   atomic.Bool
}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests_total": m.Requests.Load(),
		"analysis_bytes": m.AnalysisBytes.Load(), "analysis_evictions_total": m.AnalysisEvictions.Load(), "decode_errors_total": m.DecodeErrors.Load(),
		"captured_total": m.Captured.Load(), "accepted_total": m.Accepted.Load(), "filtered_total": m.Filtered.Load(), "unsupported_total": m.Unsupported.Load(), "capture_drops_total": m.CaptureDrops.Load(), "queue_drops_total": m.QueueDrops.Load(), "flow_drops_total": m.FlowDrops.Load(), "write_drops_total": m.WriteDrops.Load(), "gaps_total": m.Gaps.Load(), "active_bytes": m.ActiveBytes.Load(), "queue_bytes": m.QueueBytes.Load(), "write_bytes": m.WriteBytes.Load(), "active_flows": m.ActiveFlows.Load(), "persisted_chunks_total": m.PersistedChunks.Load(), "persisted_bytes_total": m.PersistedBytes.Load(), "db_errors_total": m.DBErrors.Load(), "last_capture_unix": m.LastCapture.Load(), "last_process_unix": m.LastProcess.Load(), "last_commit_unix": m.LastCommit.Load(), "pending_since_unix": m.PendingSince.Load(),
	}
}
func (m *Metrics) Write(w io.Writer) {
	fmt.Fprintf(w, "gomate_requests_per_second %g\n", m.RequestRate())
	for k, v := range m.Snapshot() {
		fmt.Fprintf(w, "gomate_%s %d\n", k, v)
	}
}
func (m *Metrics) Stalled() bool {
	p := m.PendingSince.Load()
	return p > 0 && time.Now().Unix()-p > 15 || m.QueueBytes.Load() > 0 && time.Now().Unix()-m.LastProcess.Load() > 15
}

// Reserve is non-blocking. Both successful admission and release have one owner.
func Reserve(v *atomic.Int64, n, limit int64) bool {
	for {
		old := v.Load()
		if n > limit-old {
			return false
		}
		if v.CompareAndSwap(old, old+n) {
			return true
		}
	}
}

// A fixed five-second window uses 100 ms buckets, independent of session and
// database retention. Recording happens once per analysis batch, not per packet.
const requestBucketWidth = 100 * time.Millisecond
const requestBucketCount = 50

type requestBucket struct {
	tick  int64
	count int64
}
type requestWindow struct {
	mu      sync.Mutex
	buckets [requestBucketCount]requestBucket
}

func (w *requestWindow) record(n int64, now time.Time) {
	tick := now.UnixNano() / int64(requestBucketWidth)
	w.mu.Lock()
	defer w.mu.Unlock()
	b := &w.buckets[tick%requestBucketCount]
	if b.tick != tick {
		*b = requestBucket{tick: tick}
	}
	b.count += n
}
func (w *requestWindow) rate(now time.Time) float64 {
	tick := now.UnixNano() / int64(requestBucketWidth)
	w.mu.Lock()
	defer w.mu.Unlock()
	var total int64
	for _, b := range w.buckets {
		if b.tick <= tick && b.tick > tick-requestBucketCount {
			total += b.count
		}
	}
	return float64(total) / (float64(requestBucketCount) * requestBucketWidth.Seconds())
}
func (m *Metrics) RecordRequests(n int64) {
	if n <= 0 {
		return
	}
	m.Requests.Add(n)
	m.requestRate.record(n, time.Now())
}
func (m *Metrics) RequestRate() float64 { return m.requestRate.rate(time.Now()) }
