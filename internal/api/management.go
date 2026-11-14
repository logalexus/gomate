package api

import (
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"gomate/internal/analyze"
	"gomate/internal/control"
	"gomate/internal/discovery"
	"gomate/internal/model"
	"gomate/internal/storage"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) management(w http.ResponseWriter, r *http.Request, p []string) (bool, error) {
	if len(p) < 3 {
		return false, nil
	}
	ctx := r.Context()
	switch p[2] {
	case "interfaces":
		if r.Method != "GET" {
			return true, &httpError{405, "method not allowed"}
		}
		v, e := control.Interfaces()
		if e != nil {
			return true, e
		}
		respond(w, 200, v)
		return true, nil
	case "settings":
		if s.Control == nil {
			return true, &httpError{409, "dynamic LIVE configuration is unavailable in PCAP mode"}
		}
		if r.Method == "GET" {
			respond(w, 200, map[string]any{"settings": s.Control.Settings(), "capture": s.Control.Status()})
			return true, nil
		}
		if r.Method != "PUT" {
			return true, &httpError{405, "method not allowed"}
		}
		var v model.RuntimeSettings
		if e := decode(w, r, &v); e != nil {
			return true, e
		}
		// Preserve the setting when an older client saves an unrelated field.
		if v.AnomalyTimeoutSeconds == 0 {
			v.AnomalyTimeoutSeconds = s.anomalyTimeoutSeconds()
		}
		if e := control.Validate(v); e != nil {
			return true, bad(e.Error())
		}
		e := s.Control.Apply(ctx, v, s.Store.PutSettings)
		if errors.Is(e, storage.ErrRevision) {
			return true, &httpError{409, e.Error()}
		}
		if e != nil {
			var pe *pgconn.PgError
			if errors.As(e, &pe) {
				return true, e
			}
			return true, bad(e.Error())
		}
		respond(w, 200, map[string]any{"settings": s.Control.Settings(), "capture": s.Control.Status()})
		return true, nil
	case "docker-services":
		if r.Method != "GET" && r.Method != "POST" {
			return true, &httpError{405, "method not allowed"}
		}
		candidates, e := discovery.Fetch(ctx, s.Config.DiscoveryURL, s.Config.DiscoveryToken)
		if e != nil {
			return true, &httpError{503, e.Error()}
		}
		if r.Method == "GET" {
			respond(w, 200, candidates)
			return true, nil
		}
		var body struct {
			ID string `json:"id"`
		}
		if e := decode(w, r, &body); e != nil {
			return true, e
		}
		var selected *discovery.Candidate
		for i := range candidates {
			if candidates[i].ID == body.ID {
				selected = &candidates[i]
				break
			}
		}
		if selected == nil || !selected.Available {
			return true, &httpError{409, "container or published port changed; refresh discovery"}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		v := model.Service{Port: selected.Port, Name: selected.Name}
		if e = s.Store.CreateService(ctx, v); e != nil {
			if errors.Is(e, storage.ErrServiceExists) {
				return true, &httpError{409, "this port is already being analyzed"}
			}
			return true, e
		}
		if e = s.Reload(ctx); e != nil {
			return true, e
		}
		s.Hub.Notify()
		respond(w, 201, v)
		return true, nil

	case "cleanup":
		if r.Method == "GET" {
			v, e := s.Store.CleanupJobs(ctx)
			if e != nil {
				return true, e
			}
			respond(w, 200, v)
			return true, nil
		}
		if len(p) == 4 && r.Method == "DELETE" {
			id, e := strconv.ParseInt(p[3], 10, 64)
			if e != nil {
				return true, bad("invalid cleanup id")
			}
			if e = s.Store.CancelCleanup(ctx, id); e != nil {
				return true, e
			}
			w.WriteHeader(204)
			return true, nil
		}
		if r.Method != "POST" {
			return true, &httpError{405, "method not allowed"}
		}
		var body struct {
			Mode              string `json:"mode"`
			PreserveFavorites *bool  `json:"preserveFavorites"`
			Service           int    `json:"service"`
			Confirm           string `json:"confirm"`
		}
		if e := decode(w, r, &body); e != nil {
			return true, e
		}
		if body.Service < 0 || body.Service > 65535 {
			return true, bad("invalid service")
		}
		keep := body.PreserveFavorites == nil || *body.PreserveFavorites
		if len(p) == 4 && p[3] == "preview" {
			v, e := s.Store.CleanupPreview(ctx, keep, body.Service)
			if e != nil {
				return true, e
			}
			respond(w, 200, v)
			return true, nil
		}
		if body.Mode != "packets" && body.Mode != "sessions" {
			return true, bad("mode must be packets or sessions")
		}
		if body.Confirm != "DELETE" {
			return true, bad("explicit deletion confirmation is required")
		}
		id, e := s.Store.StartCleanup(ctx, body.Mode, keep, body.Service)
		if e != nil {
			var pe *pgconn.PgError
			if errors.As(e, &pe) && pe.Code == "23505" {
				return true, &httpError{409, "another cleanup is active"}
			}
			return true, e
		}
		respond(w, 202, map[string]int64{"id": id})
		return true, nil
	case "pattern-test":
		if r.Method != "POST" {
			return true, &httpError{405, "method not allowed"}
		}
		var body struct {
			Pattern  model.Pattern `json:"pattern"`
			Text     string        `json:"text"`
			Hex      bool          `json:"hex"`
			Incoming bool          `json:"incoming"`
		}
		if e := decode(w, r, &body); e != nil {
			return true, e
		}
		if len(body.Text) > 16384 {
			return true, bad("sample must not exceed 16 KiB")
		}
		body.Pattern.ID = 1
		body.Pattern.Enabled = true
		body.Pattern.Deleted = false
		if e := analyze.Validate(body.Pattern); e != nil {
			return true, bad(e.Error())
		}
		b := []byte(body.Text)
		if body.Hex {
			var e error
			b, e = hex.DecodeString(strings.Join(strings.Fields(body.Text), ""))
			if e != nil {
				return true, bad("sample is not valid hex")
			}
		}
		cat := analyze.NewCatalog()
		cat.Replace(nil, []model.Pattern{body.Pattern})
		port := 0
		if body.Pattern.ServiceID != nil {
			port = *body.Pattern.ServiceID
		}
		c := model.Chunk{Stream: model.Stream{Service: port}, Packets: []model.Packet{{Content: b, Incoming: body.Incoming}}}
		cat.Analyze(&c)
		respond(w, 200, map[string]any{"valid": true, "matches": c.Packets[0].Matches, "ignored": c.Ignore, "incomplete": c.Packets[0].Incomplete})
		return true, nil
	case "patterns-bulk":
		if r.Method != "POST" {
			return true, &httpError{405, "method not allowed"}
		}
		var body struct {
			IDs    []int  `json:"ids"`
			Action string `json:"action"`
		}
		if e := decode(w, r, &body); e != nil {
			return true, e
		}
		if len(body.IDs) == 0 || len(body.IDs) > 1000 {
			return true, bad("select 1–1000 patterns")
		}
		if body.Action != "enable" && body.Action != "disable" && body.Action != "delete" {
			return true, bad("unknown bulk action")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if body.Action == "enable" {
			all, e := s.Store.Patterns(ctx)
			if e != nil {
				return true, e
			}
			ids := map[int]bool{}
			for _, id := range body.IDs {
				ids[id] = true
			}
			for _, p := range all {
				if ids[p.ID] {
					if e := analyze.Validate(p); e != nil {
						return true, bad(e.Error())
					}
				}
			}
		}
		if e := s.Store.PatternsBulk(ctx, body.IDs, body.Action); e != nil {
			return true, e
		}
		if e := s.Reload(ctx); e != nil {
			return true, e
		}
		s.Hub.Notify()
		w.WriteHeader(204)
		return true, nil
	}
	return false, nil
}
