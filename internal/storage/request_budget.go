package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type requestBudgetKey struct{}

// WithRequestBudget lets interactive SQL use the HTTP deadline. Background
// capture and maintenance keep their independent, short database limits.
func WithRequestBudget(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestBudgetKey{}, true)
}

func configureRequestBudgets(cfg *pgxpool.Config) {
	// A connection is exclusively owned during PrepareConn. Track only those
	// requiring a reset, avoiding an extra round trip on ordinary writer reads.
	var changed sync.Map
	cfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		statement, lock := "3000ms", "1000ms"
		interactive, _ := ctx.Value(requestBudgetKey{}).(bool)
		if interactive {
			deadline, ok := ctx.Deadline()
			if !ok {
				return true, fmt.Errorf("request budget requires a deadline")
			}
			if err := ctx.Err(); err != nil {
				return true, err
			}
			statement = fmt.Sprintf("%dms", max(int64(1), time.Until(deadline).Milliseconds()))
			lock = statement
		} else if _, ok := changed.Load(conn); !ok {
			return true, nil
		}
		_, err := conn.Exec(ctx, "SELECT set_config('statement_timeout',$1,false), set_config('lock_timeout',$2,false)", statement, lock)
		if err != nil {
			return false, err
		}
		if interactive {
			changed.Store(conn, true)
		} else {
			changed.Delete(conn)
		}
		return true, nil
	}
	cfg.BeforeClose = func(conn *pgx.Conn) { changed.Delete(conn) }
}
