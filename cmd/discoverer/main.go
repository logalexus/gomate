// Fixed-purpose Docker gateway: discovery, bounded logs, lifecycle actions and
// disposable script containers. No generic Docker endpoint is exposed.
package main

import (
	"context"
	"encoding/json"
	"gomate/internal/discovery"
	"gomate/internal/dockerui"
	"gomate/internal/monitor"
	"gomate/internal/runner"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	token := os.Getenv("DOCKER_DISCOVERY_TOKEN")
	if len(token) < 24 {
		log.Fatal("DOCKER_DISCOVERY_TOKEN must contain at least 24 characters")
	}
	socket := os.Getenv("DOCKER_SOCKET")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	mux := http.NewServeMux()
	engine := dockerui.New(socket)
	proc, root := os.Getenv("MONITOR_HOST_PROC"), os.Getenv("MONITOR_HOST_ROOT")
	if proc == "" {
		proc = "/host/proc"
	}
	if root == "" {
		root = "/host/root"
	}
	resources := monitor.New(engine, proc, root)
	go resources.Run(context.Background())
	mux.Handle("GET /monitor", resources)
	manager := runner.New(socket, os.Getenv("RUNNER_IMAGE"))
	startup, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	if err := manager.Initialize(startup); err != nil {
		log.Printf("script runner disabled: orphan cleanup failed: %v", err)
	}
	cancelStartup()
	mux.Handle("/runner", manager.Handler())
	mux.Handle("/runner/", manager.Handler())
	mux.HandleFunc("GET /services", func(w http.ResponseWriter, r *http.Request) {
		v, e := discovery.Docker(r.Context(), socket)
		if e != nil {
			http.Error(w, "cannot list Docker containers", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(v)
	})
	listen := os.Getenv("DISCOVERY_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:65001"
	}
	s := &http.Server{Addr: listen, Handler: dockerui.Gateway(engine, token, mux), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 305 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	log.Fatal(s.ListenAndServe())
}
