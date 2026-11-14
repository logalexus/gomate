package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gomate/internal/storage"
)

// Observe the actual PostgreSQL setting immediately before the descriptor scan.
// This catches a hidden SQL ceiling even when an empty test dataset is fast.
type behaviorTimeoutTrace struct {
	observed string
	err      error
}

func (t *behaviorTimeoutTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT s.id,s.service,s.protocol,s.start_ms,") {
		t.err = conn.QueryRow(ctx, "SHOW statement_timeout").Scan(&t.observed)
	}
	return ctx
}
func (*behaviorTimeoutTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestBehaviorTimeoutUsesRequestBudgetAndRestoresPool(t *testing.T) {
	original := database(t)
	cfg := original.Pool.Config()
	trace := &behaviorTimeoutTrace{}
	cfg.ConnConfig.Tracer = trace
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &storage.Store{Pool: pool}
	for _, budget := range []time.Duration{30 * time.Second, 5 * time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		_, err = s.BehaviorGroups(ctx, storage.TrafficGroupQuery{Limit: 100})
		cancel()
		if err != nil || trace.err != nil {
			t.Fatal(err, trace.err)
		}
		observed, err := time.ParseDuration(trace.observed)
		if err != nil || observed > budget || observed < budget-time.Second {
			t.Fatalf("SQL limit %q, wanted remaining %s", trace.observed, budget)
		}
		var after string
		if err = pool.QueryRow(context.Background(), "SHOW statement_timeout").Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != "3s" {
			t.Fatalf("read timeout leaked into pooled connections: %s", after)
		}
	}
}
