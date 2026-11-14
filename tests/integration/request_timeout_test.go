package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gomate/internal/analyze"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/control"
	"gomate/internal/events"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"gomate/internal/storage"
)

type requestBudgetTrace struct {
	limits []time.Duration
	err    error
}

func (t *requestBudgetTrace) TraceQueryStart(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "gomate_") && !strings.Contains(data.SQL, "set_config") {
		var setting string
		t.err = c.QueryRow(ctx, "SHOW statement_timeout").Scan(&setting)
		if t.err == nil {
			var duration time.Duration
			duration, t.err = time.ParseDuration(setting)
			t.limits = append(t.limits, duration)
		}
	}
	return ctx
}
func (*requestBudgetTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestCommonRequestTimeoutReachesAllSQLAndRestoresWriter(t *testing.T) {
	original := database(t)
	cfg := original.Pool.Config()
	cfg.MaxConns = 1
	trace := &requestBudgetTrace{}
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := &storage.Store{Pool: pool}
	app := &api.Server{Store: store, Config: config.Config{Login: "test", Password: "test", DBTimeout: time.Second}, Catalog: analyze.NewCatalog(), Metrics: &metrics.Metrics{}, Hub: events.New()}
	app.Control = control.New(context.Background(), app.Config, app.Catalog, app.Metrics, nil, func() {})
	for _, seconds := range []int64{30, 5} {
		err = app.Control.Apply(context.Background(), model.RuntimeSettings{RequestTimeoutSeconds: seconds, CleanupIntervalSeconds: 5}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []struct {
			method, path, body string
			code               int
		}{
			{"GET", "/api/v2/snapshot", "", 200},
			{"GET", "/api/v2/chat", "", 200},
			{"GET", "/api/v2/cleanup", "", 200},
			{"GET", "/api/v2/jobs", "", 200},
			{"POST", "/api/stream/all", `{"pageSize":25}`, 200},
			{"POST", "/api/service/", `{"port":80,"name":"test"}`, 200},
		} {
			trace.limits = nil
			r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			r.SetBasicAuth("test", "test")
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Code != route.code || trace.err != nil || len(trace.limits) == 0 {
				t.Fatalf("%s: %d %s limits=%v err=%v", route.path, w.Code, w.Body.String(), trace.limits, trace.err)
			}
			for _, limit := range trace.limits {
				if limit > time.Duration(seconds)*time.Second || limit < time.Duration(seconds-1)*time.Second {
					t.Fatalf("%s SQL budget: %s", route.path, limit)
				}
			}
			var statement, lock string
			if err = pool.QueryRow(context.Background(), "SELECT current_setting('statement_timeout'),current_setting('lock_timeout')").Scan(&statement, &lock); err != nil {
				t.Fatal(err)
			}
			if statement != "3s" || lock != "1s" {
				t.Fatalf("request leaked into background connection: %s/%s", statement, lock)
			}
		}
	}
	// Actual work exceeding the old SQL ceiling succeeds with an interactive budget.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = pool.Exec(storage.WithRequestBudget(ctx), "SELECT pg_sleep(3.2)"); err != nil {
		t.Fatal(err)
	}
	ctxShort, cancelShort := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelShort()
	if _, err = pool.Exec(storage.WithRequestBudget(ctxShort), "SELECT pg_sleep(1)"); err == nil {
		t.Fatal("expired request was not cancelled")
	}
}

func TestCommonTimeoutSettingsMigrationAndSave(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	saved, err := store.InitSettings(ctx, model.RuntimeSettings{CleanupIntervalSeconds: 5, AnomalyTimeoutSeconds: 45})
	if err != nil {
		t.Fatal(err)
	}
	app := &api.Server{Store: store, Config: config.Config{Login: "test", Password: "test"}}
	app.Control = control.New(ctx, app.Config, analyze.NewCatalog(), &metrics.Metrics{}, nil, func() {})
	if err = app.Control.Apply(ctx, saved, nil); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"new", "omitted", "legacy"} {
		v := app.Control.Settings()
		want := int64(30)
		switch mode {
		case "new":
			v.RequestTimeoutSeconds = 30
		case "omitted":
			v.RequestTimeoutSeconds = 0
			v.AnomalyTimeoutSeconds = 0
		case "legacy":
			v.RequestTimeoutSeconds = 0
			v.AnomalyTimeoutSeconds = 60
			want = 60
		}
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("PUT", "/api/v2/settings", strings.NewReader(string(body)))
		r.SetBasicAuth("test", "test")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", mode, w.Code, w.Body.String())
		}
		saved, err = store.Settings(ctx)
		if err != nil || saved.RequestTimeoutSeconds != want || saved.AnomalyTimeoutSeconds != want || app.Control.Settings().RequestTimeout() != want {
			t.Fatalf("%s: saved=%+v err=%v", mode, saved, err)
		}
	}
}
