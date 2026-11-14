package api

import (
	"context"
	"runtime"
	"time"
)

func (s *Server) diagnostics(ctx context.Context) map[string]any {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	pool := s.Store.Pool.Stat()
	var dbBytes int64
	err := s.Store.Pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&dbBytes)
	out := map[string]any{"timestamp": time.Now().UTC(), "metrics": s.Metrics.Snapshot(), "goroutines": runtime.NumGoroutine(), "heapAllocBytes": mem.HeapAlloc, "heapSysBytes": mem.HeapSys, "gcCycles": mem.NumGC, "gcPauseTotalNs": mem.PauseTotalNs, "poolAcquired": pool.AcquiredConns(), "poolIdle": pool.IdleConns(), "poolMax": pool.MaxConns(), "poolAcquireWaitNs": pool.EmptyAcquireWaitTime().Nanoseconds(), "databaseBytes": dbBytes}
	if err != nil {
		out["databaseUnavailable"] = true
	}
	return out
}
