package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProtocols(t *testing.T) {
	for _, protocol := range []string{"http", "http-keepalive", "tcp", "grpc", "grpc-stream"} {
		t.Run(protocol, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			serverProtocol := protocol
			if protocol == "http-keepalive" {
				serverProtocol = "http"
			}
			if protocol == "grpc-stream" {
				serverProtocol = "grpc"
			}
			go func() { done <- serveListener(ctx, serverProtocol, l) }()
			defer func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("server failed to stop")
				}
			}()
			call, closeClient, err := client(protocol, l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer closeClient()
			request, stop := context.WithTimeout(ctx, 4*time.Second)
			defer stop()
			n, err := call(request, "test_marker")
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if protocol == "grpc-stream" {
				want = 3
			}
			if n != want {
				t.Fatalf("messages=%d expected=%d", n, want)
			}
		})
	}
}
