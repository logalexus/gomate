package dockerui

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStatsUsesOneShotBoundedResponse(t *testing.T) {
	calls := 0
	engine := &Engine{client: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/containers/"+strings.Repeat("a", 64)+"/stats" || r.URL.Query().Get("stream") != "false" || r.URL.Query().Get("one-shot") != "true" {
			t.Errorf("unexpected Docker stats request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"cpu_stats":{"online_cpus":8},"memory_stats":{"usage":1234},"networks":{"eth0":{"rx_bytes":20}}}`)), Header: make(http.Header)}, nil
	})}}
	s, err := engine.Stats(context.Background(), strings.Repeat("a", 64))
	if err != nil || s.CPU.CPUs != 8 || s.Memory.Usage != 1234 || s.Networks["eth0"].RX != 20 {
		t.Fatal(s, err)
	}
	if _, err = engine.Stats(context.Background(), "../bad"); err == nil || calls != 1 {
		t.Fatal("invalid container ID reached daemon")
	}
}
