package main

import (
	"context"
	"errors"
	"gomate/internal/anomaly"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/control"
	"gomate/internal/model"
	"gomate/internal/pipeline"
	"log/slog"
	"net/http"
	"time"
)

func serveDynamic(parent context.Context, cfg config.Config, app *api.Server) error {
	ctx, stop := context.WithCancel(parent)
	defer stop()
	work, abort := context.WithCancel(context.Background())
	defer abort()
	writer := pipeline.New(work, cfg, app.Metrics, app.Catalog, app.Store, app.Hub.Notify)
	detector := anomaly.New(app.Catalog, app.Store, cfg.DBTimeout)
	writer.Observe(detector.Submit)
	app.Anomaly = detector
	manager := control.New(ctx, cfg, app.Catalog, app.Metrics, writer.Submit, app.Hub.Notify)
	app.Control = manager
	defaults := model.RuntimeSettings{CaptureEnabled: cfg.Interface != "", Interface: cfg.Interface, AutoIPs: len(cfg.LocalIPs) == 0, LocalIPs: []string{}, SessionRetentionSeconds: int64(cfg.Retention / time.Second), CleanupIntervalSeconds: 5, PreserveFavorites: true}
	for _, ip := range cfg.LocalIPs {
		defaults.LocalIPs = append(defaults.LocalIPs, ip.String())
	}
	q, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
	settings, e := app.Store.InitSettings(q, defaults)
	cancel()
	if e != nil {
		writer.Close()
		return e
	}
	manager.Startup(settings)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		lastRetention := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				q, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
				changed, e := app.Store.StepCleanup(q)
				cancel()
				if e != nil && ctx.Err() == nil {
					slog.Warn("manual cleanup will retry", "error", e)
				}
				if changed {
					app.Hub.Notify()
				}
				settings := manager.Settings()
				if time.Since(lastRetention) >= time.Duration(settings.CleanupIntervalSeconds)*time.Second {
					q, cancel = context.WithTimeout(ctx, cfg.DBTimeout)
					e = app.Store.Retain(q, settings)
					cancel()
					lastRetention = time.Now()
					if e != nil && ctx.Err() == nil {
						slog.Warn("retention will retry", "error", e)
					}
				}
				if app.Metrics.Stalled() || app.Metrics.WriteBytes.Load() > cfg.WriteBytes/2 {
					continue
				}
				q, cancel = context.WithTimeout(ctx, cfg.DBTimeout)
				changed, e = app.Store.StepLookback(q)
				cancel()
				if e != nil && ctx.Err() == nil {
					slog.Warn("lookback will retry", "error", e)
				}
				if changed {
					app.Hub.Notify()
				}
			}
		}
	}()
	server := &http.Server{Addr: cfg.Listen, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	slog.Info("gomate started with dynamic configuration", "listen", cfg.Listen, "interface", settings.Interface)
	var result error
	select {
	case <-ctx.Done():
	case e = <-serveErr:
		if !errors.Is(e, http.ErrServerClosed) {
			result = e
		}
	}
	stop()
	deadline := time.AfterFunc(10*time.Second, abort)
	defer deadline.Stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	server.Shutdown(shutdown)
	cancel()
	manager.Close()
	writer.Close()
	detector.Close()
	<-workerDone
	slog.Info("shutdown complete", "metrics", app.Metrics.Snapshot())
	return result
}
