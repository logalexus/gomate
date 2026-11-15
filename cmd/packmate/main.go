package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/anomaly"
	"gomate/internal/api"
	"gomate/internal/capture"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/farm"
	"gomate/internal/flow"
	"gomate/internal/metrics"
	"gomate/internal/pipeline"
	"gomate/internal/storage"
)

func main() {
	if e := run(); e != nil {
		slog.Error("packmate stopped", "error", e)
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	store, e := storage.Open(startup, cfg.DatabaseURL)
	cancel()
	if e != nil {
		return e
	}
	defer store.Close()
	farmService, e := farm.New(ctx, store.Pool)
	if e != nil {
		return e
	}
	defer farmService.Close()
	m := &metrics.Metrics{}
	catalog := analyze.NewCatalog()
	hub := events.New()
	app := &api.Server{Farm: farmService, Config: cfg, Store: store, Catalog: catalog, Metrics: m, Hub: hub}
	reload, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
	e = app.Reload(reload)
	cancel()
	if e != nil {
		return e
	}
	if cfg.PCAP == "" {
		return serveDynamic(ctx, cfg, app)
	}
	// Keep writer alive while capture drains. The separate cancellation below
	// imposes the shutdown deadline without discarding all queued work at SIGTERM.
	work, abort := context.WithCancel(context.Background())
	defer abort()
	writer := pipeline.New(work, cfg, m, catalog, store, hub.Notify)
	detector := anomaly.New(catalog, store, cfg.DBTimeout)
	defer detector.Close()
	writer.Observe(detector.Submit)
	app.Anomaly = detector
	engine := flow.New(cfg, catalog, m, cfg.PCAP == "", writer.Submit)
	server := &http.Server{Addr: cfg.Listen, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	captureDone := make(chan error, 1)
	go func() {
		var err error
		if cfg.Interface != "" {
			err = capture.Live(ctx, cfg.Interface, engine, m)
		} else if cfg.PCAP != "" {
			err = capture.File(ctx, cfg.PCAP, engine, m)
		} else {
			<-ctx.Done()
		}
		captureDone <- err
	}()
	jobsDone := make(chan struct{})
	go func() {
		defer close(jobsDone)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if m.Stalled() || m.WriteBytes.Load() > cfg.WriteBytes/2 {
					continue
				}
				q, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
				_, err := store.StepBehaviorBackfill(q)
				cancel()
				if err != nil && ctx.Err() == nil {
					slog.Warn("behavior backfill will retry", "error", err)
				}
				q, cancel = context.WithTimeout(ctx, cfg.DBTimeout)
				changed, err := store.StepLookback(q)
				cancel()
				if err != nil && ctx.Err() == nil {
					slog.Warn("lookback step failed; will retry", "error", err)
				}
				if changed {
					hub.Notify()
				}
			}
		}
	}()
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if cfg.PCAP != "" {
					continue
				}
				q, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
				err := store.Cleanup(q, time.Now().Add(-cfg.Retention))
				cancel()
				if err != nil {
					slog.Warn("retention cleanup failed", "error", err)
				}
			}
		}
	}()
	slog.Info("packmate started", "listen", cfg.Listen, "interface", cfg.Interface, "shards", cfg.Shards)
	var result error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
		stop()
	case err := <-captureDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			result = err
			stop()
		} else if cfg.PCAP != "" {
			slog.Info("PCAP read complete; draining")
		}
		captureDone = nil
		if cfg.PCAP != "" && result == nil {
			engine.Close()
			if err := writer.Close(); err != nil {
				result = err
				stop()
			}
			slog.Info("PCAP drain finished", "metrics", m.Snapshot())
			engine = nil
			writer = nil
			select {
			case <-ctx.Done():
			case err = <-serveErr:
				result = err
				stop()
			}
		}
	}
	stop()
	deadline := time.AfterFunc(10*time.Second, abort)
	defer deadline.Stop()
	if captureDone != nil {
		<-captureDone
	}
	if engine != nil {
		engine.Close()
	}
	if writer != nil {
		writer.Close()
	}
	<-cleanupDone
	<-jobsDone
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server.Shutdown(shutdown)
	slog.Info("shutdown complete", "metrics", m.Snapshot())
	return result
}
