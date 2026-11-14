package metrics

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

type Metrics struct {
	AnalysisBytes, AnalysisEvictions, DecodeErrors                                                   atomic.Int64
	Captured, Accepted, Filtered, Unsupported, CaptureDrops, QueueDrops, FlowDrops, WriteDrops, Gaps atomic.Int64
	ActiveBytes, QueueBytes, WriteBytes, ActiveFlows, PersistedChunks, PersistedBytes, DBErrors      atomic.Int64
	LastCapture, LastProcess, LastCommit, PendingSince                                               atomic.Int64
	CaptureRunning                                                                                   atomic.Bool
}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"analysis_bytes": m.AnalysisBytes.Load(), "analysis_evictions_total": m.AnalysisEvictions.Load(), "decode_errors_total": m.DecodeErrors.Load(),
		"captured_total": m.Captured.Load(), "accepted_total": m.Accepted.Load(), "filtered_total": m.Filtered.Load(), "unsupported_total": m.Unsupported.Load(), "capture_drops_total": m.CaptureDrops.Load(), "queue_drops_total": m.QueueDrops.Load(), "flow_drops_total": m.FlowDrops.Load(), "write_drops_total": m.WriteDrops.Load(), "gaps_total": m.Gaps.Load(), "active_bytes": m.ActiveBytes.Load(), "queue_bytes": m.QueueBytes.Load(), "write_bytes": m.WriteBytes.Load(), "active_flows": m.ActiveFlows.Load(), "persisted_chunks_total": m.PersistedChunks.Load(), "persisted_bytes_total": m.PersistedBytes.Load(), "db_errors_total": m.DBErrors.Load(), "last_capture_unix": m.LastCapture.Load(), "last_process_unix": m.LastProcess.Load(), "last_commit_unix": m.LastCommit.Load(), "pending_since_unix": m.PendingSince.Load(),
	}
}
func (m *Metrics) Write(w io.Writer) {
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
