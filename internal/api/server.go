package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"gomate/internal/analyze"
	"gomate/internal/anomaly"
	"gomate/internal/arena"
	"gomate/internal/config"
	"gomate/internal/control"
	"gomate/internal/events"
	"gomate/internal/farm"
	"gomate/internal/metrics"
	"gomate/internal/model"
	"gomate/internal/storage"
)

type Server struct {
	Farm          *farm.Service
	arenaGame     arena.Hub
	chatLimit     chatLimiter
	behaviorPages behaviorPageCache
	Control       *control.Manager
	Config        config.Config
	Store         *storage.Store
	Catalog       *analyze.Catalog
	Metrics       *metrics.Metrics
	Hub           *events.Hub
	Anomaly       *anomaly.Engine
	mu            sync.Mutex
}

func (s *Server) Reload(ctx context.Context) error {
	ss, e := s.Store.Services(ctx)
	if e != nil {
		return e
	}
	ps, e := s.Store.Patterns(ctx)
	if e != nil {
		return e
	}
	return s.Catalog.Replace(ss, ps)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health/live" {
		respond(w, 200, map[string]bool{"alive": true})
		return
	}
	user, pass, ok := r.BasicAuth()
	u, p := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(pass))
	wantU, wantP := sha256.Sum256([]byte(s.Config.Login)), sha256.Sum256([]byte(s.Config.Password))
	if !ok || subtle.ConstantTimeCompare(u[:], wantU[:])&subtle.ConstantTimeCompare(p[:], wantP[:]) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="gomate"`)
		http.Error(w, "authentication required", 401)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	if r.Method != "GET" && r.Method != "HEAD" {
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
			http.Error(w, "invalid origin", 403)
			return
		}
	}
	if r.URL.Path == "/api/v2/chat/arena" {
		s.arenaGame.Serve(w, r, s.chatAuthor(w, r))
		return
	}
	if r.URL.Path == "/api/v2/events" {
		s.Hub.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/metrics" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		s.Metrics.Write(w)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/health/ready" {
		s.static(w, r)
		return
	}
	// Discover the common UI/API budget without depending on a database query.
	if (r.URL.Path == "/api/v2/config" || r.URL.Path == "/api/v2/anomalies/config") && r.Method == "GET" {
		w.Header().Set("Cache-Control", "no-store")
		respond(w, 200, map[string]int64{"timeoutSeconds": s.requestTimeoutSeconds()})
		return
	}
	// Return only the already authenticated caller's credentials, on explicit UI request.
	if r.URL.Path == "/api/v2/farm/client-credentials" {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		respond(w, http.StatusOK, map[string]string{"username": user, "password": pass})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.requestTimeoutSeconds())*time.Second)
	defer cancel()
	ctx = storage.WithRequestBudget(ctx)
	r = r.WithContext(ctx)
	if strings.HasPrefix(r.URL.Path, "/api/v2/farm/") && s.Farm != nil {
		s.Farm.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/api/v2/monitor" {
		if e := s.monitor(w, r); e != nil {
			s.fail(w, e)
		}
		return
	}
	// Script execution limits are separate; their HTTP control requests share the API budget.
	if r.URL.Path == "/api/v2/runner" || strings.HasPrefix(r.URL.Path, "/api/v2/runner/") {
		if e := s.runner(w, r); e != nil {
			s.fail(w, e)
		}
		return
	}
	// Container proxy calls inherit the common request budget.
	if strings.HasPrefix(r.URL.Path, "/api/v2/containers") {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 || parts[2] != "containers" {
			http.NotFound(w, r)
			return
		}
		if e := s.containers(w, r, parts); e != nil {
			s.fail(w, e)
		}
		return
	}
	if r.URL.Path == "/health/ready" {
		e := s.Store.Pool.Ping(ctx)
		captureReady := s.Config.Interface == "" || s.Metrics.CaptureRunning.Load()
		if s.Control != nil {
			captureReady = !s.Control.Settings().CaptureEnabled || s.Control.Status().Running
		}
		ready := e == nil && !s.Metrics.Stalled() && captureReady
		status := 200
		if !ready {
			status = 503
		}
		respond(w, status, map[string]bool{"ready": ready})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	var err error
	switch parts[1] {
	case "v2":
		if len(parts) >= 3 && parts[2] == "anomalies" {
			if e := s.anomalies(w, r, parts); e != nil {
				s.fail(w, e)
			}
			return
		}
		if len(parts) == 3 && parts[2] == "chat" {
			if e := s.chat(w, r); e != nil {
				s.fail(w, e)
			}
			return
		}
		if handled, e := s.management(w, r, parts); handled {
			if e != nil {
				s.fail(w, e)
			}
			return
		}
		if len(parts) == 3 && parts[2] == "diagnostics" && r.Method == "GET" {
			respond(w, 200, s.diagnostics(ctx))
			return
		}
		if len(parts) == 3 && parts[2] == "jobs" && r.Method == "GET" {
			v, e := s.Store.Jobs(ctx)
			if e != nil {
				err = e
				break
			}
			respond(w, 200, v)
			return
		}
		if len(parts) == 4 && parts[2] == "jobs" && r.Method == "DELETE" {
			id, e := strconv.ParseInt(parts[3], 10, 64)
			if e != nil || id < 1 {
				err = bad("invalid job id")
				break
			}
			err = s.Store.CancelLookback(ctx, id)
			if err == nil {
				w.WriteHeader(204)
				return
			}
			break
		}

		if len(parts) == 3 && parts[2] == "snapshot" && r.Method == "GET" {
			ss, e := s.Store.Services(ctx)
			if e != nil {
				err = e
				break
			}
			ps, e := s.Store.Patterns(ctx)
			if e != nil {
				err = e
				break
			}
			var captureStatus any
			if s.Control != nil {
				captureStatus = s.Control.Status()
			}
			respond(w, 200, map[string]any{"capture": captureStatus, "services": ss, "patterns": ps, "metrics": s.Metrics.Snapshot(), "captureRunning": s.Metrics.CaptureRunning.Load(), "stalled": s.Metrics.Stalled(), "capabilities": map[string]bool{"rawTCP": true, "udp": true, "httpDecode": true, "grpcDecode": true, "lookback": true}})
			return
		}
	case "service":
		err = s.services(w, r, parts)
		if err == nil {
			return
		}
	case "pattern":
		err = s.patterns(w, r, parts)
		if err == nil {
			return
		}
	case "stream":
		if r.Method == "GET" && len(parts) == 3 {
			id, e := strconv.ParseInt(parts[2], 10, 64)
			if e != nil || id < 1 {
				err = bad("invalid stream id")
				break
			}
			var stream model.Stream
			stream, err = s.Store.Stream(ctx, id)
			if err == nil {
				respond(w, 200, stream)
				return
			}
			break
		}
		if r.Method == "POST" && len(parts) == 3 {
			var page model.Page
			if err = decode(w, r, &page); err != nil {
				break
			}
			if err = validatePage(&page); err != nil {
				break
			}
			port := 0
			if parts[2] != "all" {
				port, err = strconv.Atoi(parts[2])
				if err != nil || port < 1 || port > 65535 {
					err = bad("invalid service port")
					break
				}
			}
			var streams []model.Stream
			streams, err = s.Store.Streams(ctx, port, page)
			if err == nil {
				respond(w, 200, streams)
				return
			}
		} else if r.Method == "POST" && len(parts) == 4 && (parts[3] == "favorite" || parts[3] == "unfavorite") {
			id, e := strconv.ParseInt(parts[2], 10, 64)
			if e != nil || id < 1 {
				err = bad("invalid stream id")
				break
			}
			err = s.Store.Favorite(ctx, id, parts[3] == "favorite")
			if err == nil {
				s.Hub.Notify()
				w.WriteHeader(204)
				return
			}
		}
	case "packet":
		if r.Method == "POST" && len(parts) == 3 {
			id, e := strconv.ParseInt(parts[2], 10, 64)
			if e != nil || id < 1 {
				err = bad("invalid stream id")
				break
			}
			var page model.Page
			if err = decode(w, r, &page); err != nil {
				break
			}
			if err = validatePage(&page); err != nil {
				break
			}
			var packets []model.Packet
			var more bool
			packets, more, err = s.Store.PacketsPage(ctx, id, page)
			w.Header().Set("X-Has-More", strconv.FormatBool(more))
			if err == nil {
				respond(w, 200, packets)
				return
			}
		}
	case "pcap":
		if len(parts) == 3 && parts[2] == "started" && r.Method == "GET" {
			respond(w, 200, s.Metrics.CaptureRunning.Load())
			return
		}
		if len(parts) == 3 && parts[2] == "start" && r.Method == "POST" {
			err = &httpError{409, "capture is configured at process startup"}
		}
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.NotFound(w, r)
}
func (s *Server) services(w http.ResponseWriter, r *http.Request, p []string) error {
	if len(p) == 2 && r.Method == "GET" {
		v, e := s.Store.Services(r.Context())
		if e != nil {
			return e
		}
		respond(w, 200, v)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "POST" && (len(p) == 2 || len(p) == 3) {
		var v model.Service
		if e := decode(w, r, &v); e != nil {
			return e
		}
		if len(p) == 3 {
			n, e := strconv.Atoi(p[2])
			if e != nil {
				return bad("invalid port")
			}
			v.Port = n
		}
		if v.Port < 1 || v.Port > 65535 || len(v.Name) == 0 || len(v.Name) > 256 {
			return bad("valid name and port required")
		}
		if v.TLS {
			return &httpError{422, "TLS decryption requires session secrets and is not supported; use plaintext capture at the TLS termination point"}
		}
		if e := s.Store.PutService(r.Context(), v); e != nil {
			return e
		}
		if e := s.Reload(r.Context()); e != nil {
			return e
		}
		s.Hub.Notify()
		respond(w, 200, v)
		return nil
	}
	if len(p) == 3 && r.Method == "DELETE" {
		n, e := strconv.Atoi(p[2])
		if e != nil {
			return bad("invalid port")
		}
		if e = s.Store.DeleteService(r.Context(), n); e != nil {
			return e
		}
		if e = s.Reload(r.Context()); e != nil {
			return e
		}
		s.Hub.Notify()
		w.WriteHeader(204)
		return nil
	}
	return &httpError{405, "method not allowed"}
}
func (s *Server) patterns(w http.ResponseWriter, r *http.Request, p []string) error {
	if len(p) == 2 && r.Method == "GET" {
		v, e := s.Store.Patterns(r.Context())
		if e != nil {
			return e
		}
		respond(w, 200, v)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var v model.Pattern
	if len(p) >= 3 {
		id, e := strconv.Atoi(p[2])
		if e != nil {
			return bad("invalid pattern id")
		}
		ps, e := s.Store.Patterns(r.Context())
		if e != nil {
			return e
		}
		for _, x := range ps {
			if x.ID == id {
				v = x
				break
			}
		}
		if v.ID == 0 || v.Deleted {
			return pgx.ErrNoRows
		}
	}
	if len(p) == 3 && r.Method == "PUT" {
		if v.Deleted {
			return &httpError{409, "pattern has been deleted"}
		}
		old := v
		var next model.Pattern
		if e := decode(w, r, &next); e != nil {
			return e
		}
		next.ID = old.ID
		next.Deleted = false
		if e := analyze.Validate(next); e != nil {
			return bad(e.Error())
		}
		if e := s.Store.ReplacePattern(r.Context(), old, &next); e != nil {
			return e
		}
		if e := s.Reload(r.Context()); e != nil {
			return e
		}
		s.Hub.Notify()
		respond(w, 200, next)
		return nil
	}
	switch {
	case len(p) == 2 && r.Method == "POST":
		all, err := s.Store.Patterns(r.Context())
		if err != nil {
			return err
		}
		active := 0
		for _, pattern := range all {
			if !pattern.Deleted {
				active++
			}
		}
		if active >= 1000 {
			return &httpError{409, "maximum 1000 non-deleted patterns"}
		}
		var body struct {
			model.Pattern
			Enabled *bool `json:"enabled"`
		}
		if e := decode(w, r, &body); e != nil {
			return e
		}
		v = body.Pattern
		v.ID = 0
		v.Enabled = true
		if body.Enabled != nil {
			v.Enabled = *body.Enabled
		}
		v.Deleted = false
	case len(p) == 3 && r.Method == "POST":
		var patch struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		if e := decode(w, r, &patch); e != nil {
			return e
		}
		v.Name = patch.Name
		v.Color = patch.Color
	case len(p) == 3 && r.Method == "DELETE":
		if e := s.Store.PatternsBulk(r.Context(), []int{v.ID}, "delete"); e != nil {
			return e
		}
		if e := s.Reload(r.Context()); e != nil {
			return e
		}
		v.Deleted = true
		s.Hub.Notify()
		respond(w, 200, v)
		return nil
	case len(p) == 4 && p[3] == "enable" && r.Method == "POST":
		enabled, e := strconv.ParseBool(r.URL.Query().Get("enabled"))
		if e != nil {
			return bad("enabled must be boolean")
		}
		v.Enabled = enabled
	case len(p) == 4 && p[3] == "lookback" && r.Method == "POST":
		if e := analyze.Validate(v); e != nil {
			return bad(e.Error())
		}
		var minutes int
		if e := decode(w, r, &minutes); e != nil {
			return e
		}
		if minutes < 1 || minutes > 10080 {
			return bad("minutes must be between 1 and 10080")
		}
		id, e := s.Store.StartLookback(r.Context(), v, time.Now().Add(-time.Duration(minutes)*time.Minute))
		if e != nil {
			var pe *pgconn.PgError
			if errors.As(e, &pe) && pe.Code == "23505" {
				return &httpError{409, "another lookback is active; wait or cancel it"}
			}
			return e
		}
		respond(w, 202, map[string]int64{"id": id})
		return nil
	default:
		return &httpError{405, "method not allowed"}
	}
	if e := analyze.Validate(v); e != nil {
		return bad(e.Error())
	}
	if e := s.Store.PutPattern(r.Context(), &v); e != nil {
		return e
	}
	if e := s.Reload(r.Context()); e != nil {
		return e
	}
	s.Hub.Notify()
	respond(w, 200, v)
	return nil
}

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }
func bad(s string) error           { return &httpError{400, s} }
func (s *Server) fail(w http.ResponseWriter, e error) {
	var h *httpError
	if errors.As(e, &h) {
		respond(w, h.status, map[string]string{"error": h.message})
		return
	}
	if errors.Is(e, pgx.ErrNoRows) {
		respond(w, 404, map[string]string{"error": "not found"})
		return
	}
	slog.Error("API request failed", "error", e)
	respond(w, 503, map[string]string{"error": "storage unavailable; retry later"})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(dst); e != nil {
		return bad("invalid JSON body")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return bad("expected one JSON value")
	}
	return nil
}
func validatePage(p *model.Page) error {
	if p.Representation != "" && p.Representation != "raw" && p.Representation != "decoded" {
		return bad("representation must be raw or decoded")
	}
	if p.PageSize == 0 {
		p.PageSize = 25
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		return bad("pageSize must be between 1 and 100")
	}
	if p.StartingFrom != nil && *p.StartingFrom < 0 {
		return bad("invalid cursor")
	}
	if _, e := p.SelectedPatternIDs(); e != nil {
		return bad(e.Error())
	}
	return nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method not allowed", 405)
		return
	}
	path := filepath.Join(s.Config.WebDir, filepath.Clean("/"+r.URL.Path))
	if info, e := os.Stat(path); e == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.Config.WebDir, "index.html"))
}

func (s *Server) requestTimeoutSeconds() int64 {
	if s.Control != nil {
		return s.Control.Settings().RequestTimeout()
	}
	return (model.RuntimeSettings{}).RequestTimeout()
}
