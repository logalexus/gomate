package api

import (
	"net/http"
	"strconv"
	"time"

	"gomate/internal/anomaly"
	"gomate/internal/storage"
)

// GET  /api/v2/anomalies?service=&minScore=&period=&sort=time&dismissed=1&limit=
// GET  /api/v2/anomalies/stats
// POST /api/v2/anomalies/{streamId}/normal   learn the reasons and hide the row
// POST /api/v2/anomalies/{streamId}/restore  show the row again
func (s *Server) anomalies(w http.ResponseWriter, r *http.Request, p []string) error {
	if len(p) == 4 && p[3] == "stats" && r.Method == "GET" {
		st := anomaly.Stats{Services: []anomaly.ServiceStats{}}
		if s.Anomaly != nil {
			st = s.Anomaly.Stats()
		}
		respond(w, 200, st)
		return nil
	}
	if len(p) == 3 && r.Method == "GET" {
		q := r.URL.Query()
		var x storage.AnomalyQuery
		var e error
		if v := q.Get("service"); v != "" {
			if x.Service, e = strconv.Atoi(v); e != nil || x.Service < 0 || x.Service > 65535 {
				return bad("invalid service")
			}
		}
		if v := q.Get("minScore"); v != "" {
			if x.MinScore, e = strconv.ParseFloat(v, 64); e != nil || x.MinScore < 0 {
				return bad("invalid minScore")
			}
		}
		if v := q.Get("period"); v != "" {
			seconds, e := strconv.ParseInt(v, 10, 64)
			if e != nil || seconds < 0 {
				return bad("invalid period")
			}
			if seconds > 0 {
				x.SinceMs = time.Now().Add(-time.Duration(seconds) * time.Second).UnixMilli()
			}
		}
		x.Limit = 200
		if v := q.Get("limit"); v != "" {
			if x.Limit, e = strconv.Atoi(v); e != nil || x.Limit < 1 || x.Limit > 1000 {
				return bad("limit must be between 1 and 1000")
			}
		}
		x.ByTime = q.Get("sort") == "time"
		x.Dismissed = q.Get("dismissed") == "1"
		v, e := s.Store.Anomalies(r.Context(), x)
		if e != nil {
			return e
		}
		respond(w, 200, v)
		return nil
	}
	if len(p) == 5 && r.Method == "POST" && (p[4] == "normal" || p[4] == "restore") {
		id, e := strconv.ParseInt(p[3], 10, 64)
		if e != nil || id < 1 {
			return bad("invalid stream id")
		}
		v, e := s.Store.DismissAnomaly(r.Context(), id, p[4] == "normal")
		if e != nil {
			return e
		}
		if p[4] == "normal" && s.Anomaly != nil {
			s.Anomaly.Accept(v.Service, v.Reasons)
		}
		w.WriteHeader(204)
		return nil
	}
	return &httpError{405, "method not allowed"}
}
