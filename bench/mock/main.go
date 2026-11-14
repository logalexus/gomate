// mock implements real HTTP/1.1, line-oriented TCP, and HTTP/2 gRPC fixtures.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/coder/websocket"
	_ "google.golang.org/grpc/encoding/gzip"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var safe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

func response(s string) (string, error) {
	if !safe.MatchString(s) {
		return "", fmt.Errorf("invalid fixture marker")
	}
	return "FLAG_" + s, nil
}

type echoService interface {
	Echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
	Watch(*wrapperspb.StringValue, grpc.ServerStream) error
}
type echo struct{}

func (echo) Echo(_ context.Context, r *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	s, e := response(r.Value)
	return wrapperspb.String(s), e
}
func (echo) Watch(r *wrapperspb.StringValue, s grpc.ServerStream) error {
	for i := 0; i < 3; i++ {
		value, e := response(fmt.Sprintf("%s_%d", r.Value, i))
		if e != nil {
			return e
		}
		if e = s.SendMsg(wrapperspb.String(value)); e != nil {
			return e
		}
		select {
		case <-s.Context().Done():
			return s.Context().Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

var descriptor = grpc.ServiceDesc{ServiceName: "gomate.mock.Echo", HandlerType: (*echoService)(nil), Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: func(s any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	r := new(wrapperspb.StringValue)
	if e := decode(r); e != nil {
		return nil, e
	}
	handler := func(ctx context.Context, r any) (any, error) {
		return s.(echoService).Echo(ctx, r.(*wrapperspb.StringValue))
	}
	if interceptor == nil {
		return handler(ctx, r)
	}
	return interceptor(ctx, r, &grpc.UnaryServerInfo{Server: s, FullMethod: "/gomate.mock.Echo/Echo"}, handler)
}}}, Streams: []grpc.StreamDesc{{StreamName: "Watch", ServerStreams: true, Handler: func(s any, stream grpc.ServerStream) error {
	r := new(wrapperspb.StringValue)
	if e := stream.RecvMsg(r); e != nil {
		return e
	}
	return s.(echoService).Watch(r, stream)
}}}}

func serve(ctx context.Context, protocol, address string) error {
	listener, e := net.Listen("tcp", address)
	if e != nil {
		return e
	}
	return serveListener(ctx, protocol, listener)
}

func serveListener(ctx context.Context, protocol string, listener net.Listener) error {
	switch protocol {
	case "http":
		mux := http.NewServeMux()
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			v, e := response(r.URL.Query().Get("marker"))
			if e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintln(w, v)
		})
		mux.HandleFunc("GET /gzip", func(w http.ResponseWriter, r *http.Request) {
			v, e := response(r.URL.Query().Get("marker"))
			if e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			fmt.Fprintln(z, v)
			z.Close()
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", fmt.Sprint(b.Len()))
			w.Write(b.Bytes())
		})
		mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
			conn, e := websocket.Accept(w, r, nil)
			if e != nil {
				return
			}
			defer conn.CloseNow()
			q, c := context.WithTimeout(r.Context(), 5*time.Second)
			defer c()
			conn.SetReadLimit(4096)
			typ, b, e := conn.Read(q)
			if e != nil {
				return
			}
			v, e := response(string(b))
			if e != nil {
				return
			}
			conn.Write(q, typ, []byte(v))
			conn.Close(websocket.StatusNormalClosure, "")
		})
		s := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			q, c := context.WithTimeout(context.Background(), 3*time.Second)
			defer c()
			s.Shutdown(q)
		}()
		e := s.Serve(listener)
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	case "tcp":
		go func() { <-ctx.Done(); listener.Close() }()
		slots := make(chan struct{}, 256)
		var wg sync.WaitGroup
		defer wg.Wait()
		for {
			conn, e := listener.Accept()
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return e
			}
			select {
			case slots <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				scanner.Buffer(make([]byte, 1024), 4096)
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				for scanner.Scan() {
					v, e := response(scanner.Text())
					if e != nil {
						return
					}
					if _, e = fmt.Fprintln(conn, v); e != nil {
						return
					}
					conn.SetDeadline(time.Now().Add(5 * time.Second))
				}
			}()
		}
	case "grpc":
		s := grpc.NewServer(grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.MaxConcurrentStreams(256))
		s.RegisterService(&descriptor, echo{})
		hs := health.NewServer()
		hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(s, hs)
		go func() { <-ctx.Done(); s.Stop() }()
		return s.Serve(listener)
	default:
		listener.Close()
		return fmt.Errorf("unknown protocol %q", protocol)
	}
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "server", "server or client")
	protocol := flag.String("protocol", "http", "http, tcp, grpc or grpc-stream")
	address := flag.String("address", ":8080", "listen address or client host:port")
	count := flag.Int("count", 100, "scheduled calls")
	rps := flag.Int("rps", 100, "scheduled calls per second")
	workers := flag.Int("concurrency", 200, "concurrent call limit")
	prefix := flag.String("run", fmt.Sprintf("r%d", time.Now().UnixNano()), "unique alphanumeric run marker")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *mode == "server" {
		return serve(ctx, *protocol, *address)
	}
	if *mode != "client" || *count < 1 || *count > 10000000 || *rps < 1 || *rps > 100000 || *workers < 1 || *workers > 4096 || !safe.MatchString(*prefix) {
		return fmt.Errorf("invalid client arguments")
	}
	call, closeClient, e := client(*protocol, *address)
	if e != nil {
		return e
	}
	defer closeClient()
	var successes, failures, missed, messages atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	var mu sync.Mutex
	latencies := make([]float64, 0, *count)
	firstError := ""
	start := time.Now()
	for i := 0; i < *count; i++ {
		at := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(*rps)))
		if wait := time.Until(at); wait > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
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
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			q, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			begin := time.Now()
			n, e := call(q, fmt.Sprintf("%s_%s_%d", *prefix, *protocol, i))
			mu.Lock()
			latencies = append(latencies, float64(time.Since(begin).Microseconds())/1000)
			if e != nil && firstError == "" {
				firstError = e.Error()
			}
			mu.Unlock()
			if e != nil {
				failures.Add(1)
			} else {
				successes.Add(1)
				messages.Add(int64(n))
			}
		}(i)
	}
	wg.Wait()
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(float64(len(latencies)-1)*p)]
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"run": *prefix, "protocol": *protocol, "scheduled": *count, "rps": *rps, "success": successes.Load(), "errors": failures.Load(), "not_issued": missed.Load(), "response_messages": messages.Load(), "elapsed_seconds": time.Since(start).Seconds(), "p95_ms": percentile(.95), "p99_ms": percentile(.99), "first_error": firstError})
	if failures.Load() > 0 || missed.Load() > 0 {
		return fmt.Errorf("load not fully served")
	}
	return nil
}

func client(protocol, address string) (func(context.Context, string) (int, error), func(), error) {
	switch protocol {
	case "ws":
		return func(ctx context.Context, marker string) (int, error) {
			c, _, e := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
			if e != nil {
				return 0, e
			}
			defer c.CloseNow()
			if e = c.Write(ctx, websocket.MessageText, []byte(marker)); e != nil {
				return 0, e
			}
			_, b, e := c.Read(ctx)
			if e != nil {
				return 0, e
			}
			c.Close(websocket.StatusNormalClosure, "")
			if string(b) != "FLAG_"+marker {
				return 0, fmt.Errorf("unexpected WS response")
			}
			return 1, nil
		}, func() {}, nil
	case "http", "http-gzip":
		tr := &http.Transport{DisableKeepAlives: true, MaxConnsPerHost: 512}
		c := &http.Client{Transport: tr, Timeout: 5 * time.Second}
		return func(ctx context.Context, marker string) (int, error) {
			path := "/"
			if protocol == "http-gzip" {
				path = "/gzip"
			}
			r, e := http.NewRequestWithContext(ctx, "GET", "http://"+address+path+"?marker="+marker, nil)
			if e != nil {
				return 0, e
			}
			resp, e := c.Do(r)
			if e != nil {
				return 0, e
			}
			defer resp.Body.Close()
			b, e := io.ReadAll(io.LimitReader(resp.Body, 4096))
			if e != nil {
				return 0, e
			}
			if resp.StatusCode != 200 || strings.TrimSpace(string(b)) != "FLAG_"+marker {
				return 0, fmt.Errorf("unexpected HTTP response")
			}
			return 1, nil
		}, tr.CloseIdleConnections, nil
	case "tcp":
		return func(ctx context.Context, marker string) (int, error) {
			c, e := (&net.Dialer{}).DialContext(ctx, "tcp", address)
			if e != nil {
				return 0, e
			}
			defer c.Close()
			deadline, _ := ctx.Deadline()
			c.SetDeadline(deadline)
			if _, e = fmt.Fprintln(c, marker); e != nil {
				return 0, e
			}
			b, e := bufio.NewReader(io.LimitReader(c, 4096)).ReadString('\n')
			if e != nil {
				return 0, e
			}
			if strings.TrimSpace(b) != "FLAG_"+marker {
				return 0, fmt.Errorf("unexpected TCP response")
			}
			return 1, nil
		}, func() {}, nil
	case "grpc", "grpc-stream", "grpc-gzip":
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		if protocol == "grpc-gzip" {
			opts = append(opts, grpc.WithDefaultCallOptions(grpc.UseCompressor("gzip")))
		}
		conn, e := grpc.NewClient(address, opts...)
		if e != nil {
			return nil, nil, e
		}
		return func(ctx context.Context, marker string) (int, error) {
			if protocol == "grpc" || protocol == "grpc-gzip" {
				var out wrapperspb.StringValue
				e := conn.Invoke(ctx, "/gomate.mock.Echo/Echo", wrapperspb.String(marker), &out)
				if e != nil {
					return 0, e
				}
				if out.Value != "FLAG_"+marker {
					return 0, fmt.Errorf("unexpected gRPC response")
				}
				return 1, nil
			}
			s, e := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/gomate.mock.Echo/Watch")
			if e != nil {
				return 0, e
			}
			if e = s.SendMsg(wrapperspb.String(marker)); e != nil {
				return 0, e
			}
			if e = s.CloseSend(); e != nil {
				return 0, e
			}
			n := 0
			for {
				var out wrapperspb.StringValue
				e = s.RecvMsg(&out)
				if e == io.EOF {
					if n != 3 {
						return n, fmt.Errorf("incomplete gRPC stream")
					}
					return n, nil
				}
				if e != nil {
					return n, e
				}
				if out.Value != fmt.Sprintf("FLAG_%s_%d", marker, n) {
					return n, fmt.Errorf("unexpected gRPC stream response")
				}
				n++
			}
		}, func() { conn.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unknown protocol %q", protocol)
	}
}
