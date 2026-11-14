// Fixed-purpose Docker gateway: discovery, bounded logs and explicit lifecycle actions.
// No arbitrary engine URL, exec, filesystem or environment endpoint is exposed.
package main

import (
	"context"
	"encoding/json"
	"gomate/internal/discovery"
	"gomate/internal/dockerui"
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
	mux.HandleFunc("GET /services", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, e := discovery.Docker(ctx, socket)
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
	s := &http.Server{Addr: listen, Handler: dockerui.Gateway(dockerui.New(socket), token, mux), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	log.Fatal(s.ListenAndServe())
}
