// traffic provides a small, reproducible HTTP fixture and an open-loop generator.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	listen := flag.String("listen", "", "run fixture HTTP server on this address")
	target := flag.String("target", "http://127.0.0.1:18080", "fixture target URL")
	duration := flag.Duration("duration", 30*time.Second, "offered load duration")
	rps := flag.Int("rps", 200, "scheduled requests per second")
	agents := flag.Int("agents", 200, "logical attacker IDs")
	concurrent := flag.Int("concurrency", 200, "maximum simultaneous requests")
	keepalive := flag.Bool("keepalive", false, "reuse HTTP connections")
	prefix := flag.String("prefix", fmt.Sprintf("%d", time.Now().UnixNano()), "unique run marker")
	flag.Parse()
	if *listen != "" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "FLAG_%s_%s\n%s", r.URL.Query().Get("run"), r.URL.Query().Get("id"), strings.Repeat("x", 1024))
		})
		server := &http.Server{Addr: *listen, ReadHeaderTimeout: 2 * time.Second}
		if e := server.ListenAndServe(); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if *rps < 1 || *rps > 100000 || *agents < 1 || *concurrent < 1 || *duration <= 0 {
		fmt.Fprintln(os.Stderr, "invalid load parameters")
		os.Exit(2)
	}
	tr := &http.Transport{DisableKeepAlives: !*keepalive, MaxIdleConns: *concurrent, MaxIdleConnsPerHost: *concurrent, MaxConnsPerHost: *concurrent}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	var issued, success, failed, missed atomic.Int64
	var hist [25]atomic.Int64
	sem := make(chan struct{}, *concurrent)
	var wg sync.WaitGroup
	total := int64(duration.Seconds() * float64(*rps))
	start := time.Now()
	for i := int64(0); i < total; i++ {
		at := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(*rps)))
		if delay := time.Until(at); delay > 0 {
			time.Sleep(delay)
		}
		if time.Since(at) > 100*time.Millisecond {
			missed.Add(1)
			continue
		}
		select {
		case sem <- struct{}{}:
		default:
			missed.Add(1)
			continue
		}
		issued.Add(1)
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			u := fmt.Sprintf("%s/?run=%s&id=%d&agent=%d", strings.TrimRight(*target, "/"), *prefix, id, id%int64(*agents))
			r, e := http.NewRequestWithContext(ctx, "GET", u, nil)
			if e != nil {
				failed.Add(1)
				return
			}
			begin := time.Now()
			resp, e := client.Do(r)
			if e == nil {
				_, e = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
				if resp.StatusCode != 200 {
					e = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
			elapsed := time.Since(begin).Microseconds()
			bucket := 0
			for bucket < len(hist)-1 && elapsed > int64(1)<<bucket {
				bucket++
			}
			hist[bucket].Add(1)
			if e == nil {
				success.Add(1)
			} else {
				failed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	buckets := map[string]int64{}
	for i := range hist {
		buckets[fmt.Sprint(int64(1)<<i)] = hist[i].Load()
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"run": *prefix, "agents": *agents, "rps": *rps, "duration_seconds": duration.Seconds(), "elapsed_seconds": time.Since(start).Seconds(), "scheduled": total, "issued": issued.Load(), "success": success.Load(), "errors": failed.Load(), "not_issued": missed.Load(), "latency_us_power_of_two_buckets": buckets})
	if failed.Load() > 0 || missed.Load() > 0 {
		os.Exit(1)
	}
}
