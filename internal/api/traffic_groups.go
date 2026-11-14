package api

import (
	"net/http"
	"strconv"
	"time"

	"gomate/internal/storage"
)

// GET /api/v2/anomalies/groups?service=&period=&sort=&limit=&offset=
// GET /api/v2/anomalies/sessions?service=&period=&protocol=&method=&path=&before=&alerts=1
func (s *Server) trafficGroups(w http.ResponseWriter, r *http.Request, members bool) error {
	params := r.URL.Query()
	integer := func(name string, fallback, max int64) (int64, error) {
		v := params.Get(name)
		if v == "" {
			return fallback, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 || n > max {
			return 0, bad("invalid " + name)
		}
		return n, nil
	}
	port, err := integer("service", 0, 65535)
	if err != nil {
		return err
	}
	period, err := integer("period", 0, int64((1<<63-1)/time.Second))
	if err != nil {
		return err
	}
	limit, err := integer("limit", 100, 200)
	if err != nil {
		return err
	}
	if limit == 0 {
		return bad("limit must be between 1 and 200")
	}
	offset, err := integer("offset", 0, 1<<31-1)
	if err != nil {
		return err
	}
	x := storage.TrafficGroupQuery{Service: int(port), Limit: int(limit), Offset: int(offset), ByTime: params.Get("sort") == "time"}
	if period > 0 {
		x.SinceMs = time.Now().Add(-time.Duration(period) * time.Second).UnixMilli()
	}
	if !members {
		v, err := s.Store.TrafficGroups(r.Context(), x)
		if err != nil {
			return err
		}
		respond(w, 200, v)
		return nil
	}
	x.Protocol, x.Method, x.Path = params.Get("protocol"), params.Get("method"), params.Get("path")
	if x.Service == 0 || x.Protocol == "" || len(x.Protocol) > 64 || len(x.Method) > 32 || len(x.Path) > 2048 {
		return bad("invalid group")
	}
	x.Before, err = integer("before", 0, 1<<63-1)
	if err != nil {
		return err
	}
	x.AlertsOnly = params.Get("alerts") == "1"
	v, err := s.Store.TrafficGroupSessions(r.Context(), x)
	if err != nil {
		return err
	}
	respond(w, 200, v)
	return nil
}
