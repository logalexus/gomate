package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
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
	x := storage.TrafficGroupQuery{Service: int(port), Limit: int(limit), Offset: int(offset), ByTime: params.Get("sort") == "time", ByCount: params.Get("sort") == "count"}
	if period > 0 {
		x.SinceMs = time.Now().Add(-time.Duration(period) * time.Second).UnixMilli()
	}
	if !members {
		var v storage.TrafficGroupPage
		var err error
		if params.Get("grouping") == "behavior" {
			v, err = s.cachedBehaviorPage(r.Context(), x, period)
		} else {
			v, err = s.Store.TrafficGroups(r.Context(), x)
		}
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
	x.Fingerprint = params.Get("fingerprint")
	if x.Fingerprint != "" {
		if b, e := hex.DecodeString(x.Fingerprint); e != nil || len(b) != 16 {
			return bad("invalid fingerprint")
		}
	}
	v, err := s.Store.TrafficGroupSessions(r.Context(), x)
	if err != nil {
		return err
	}
	respond(w, 200, v)
	return nil
}

// Share a small, short-lived result cache between browser polls. Membership is
// checked against current rows, so deleted sessions cannot be reopened via it.
type cachedBehaviorPage struct {
	at   time.Time
	page storage.TrafficGroupPage
}
type behaviorPageCache struct {
	mu    sync.Mutex
	pages map[string]cachedBehaviorPage
}

func (s *Server) cachedBehaviorPage(ctx context.Context, q storage.TrafficGroupQuery, period int64) (storage.TrafficGroupPage, error) {
	c := &s.behaviorPages
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return storage.TrafficGroupPage{}, err
	}
	key := fmt.Sprintf("%d/%d/%t/%t/%d/%d", q.Service, period, q.ByTime, q.ByCount, q.Limit, q.Offset)
	if p, ok := c.pages[key]; ok && time.Since(p.at) < 5*time.Second {
		return p.page, nil
	}
	page, err := s.Store.BehaviorGroups(ctx, q)
	if err != nil {
		return page, err
	}
	if c.pages == nil {
		c.pages = map[string]cachedBehaviorPage{}
	}
	for k, v := range c.pages {
		if time.Since(v.at) >= 5*time.Second {
			delete(c.pages, k)
		}
	}
	if len(c.pages) >= 16 {
		for k := range c.pages {
			delete(c.pages, k)
			break
		}
	}
	c.pages[key] = cachedBehaviorPage{at: time.Now(), page: page}
	return page, nil
}
