// Local browser fixture: real authentication, static frontend and arena WebSockets.
// Browser tests intercept other API routes; no database or capture is started.
package main

import (
	"fmt"
	"gomate/internal/api"
	"gomate/internal/config"
	"net"
	"net/http"
)

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println("http://" + listener.Addr().String())
	server := &api.Server{Config: config.Config{Login: "arena-test", Password: "arena-test", WebDir: "web/dist"}}
	if err = http.Serve(listener, server); err != nil {
		panic(err)
	}
}
