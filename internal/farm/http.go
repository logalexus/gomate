package farm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if e := s.serve(w, r); e != nil {
		status := 500
		var bad *inputError
		if errors.As(e, &bad) {
			status = 400
		}
		if errors.Is(e, ErrConflict) {
			status = 409
		}
		msg := e.Error()
		if status == 500 {
			msg = "Farm database operation failed"
		}
		reply(w, status, map[string]string{"error": msg})
	}
}

type inputError struct{ error }

func invalid(s string) error { return &inputError{fmt.Errorf("%s", s)} }
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	if e := json.NewDecoder(r.Body).Decode(v); e != nil {
		return invalid("Invalid JSON or request exceeds 1 MiB")
	}
	return nil
}
func (s *Service) serve(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/farm")
	c, e := s.Config(ctx)
	if e != nil {
		return e
	}
	switch {
	case path == "/settings" && r.Method == "GET":
		reply(w, 200, c.Public())
	case path == "/settings" && r.Method == "PUT":
		var v Config
		if e = decode(w, r, &v); e != nil {
			return e
		}
		v, e = s.Save(ctx, v)
		if e != nil {
			if errors.Is(e, ErrConflict) {
				return e
			}
			return invalid(e.Error())
		}
		reply(w, 200, v.Public())
	case path == "/targets/preview" && r.Method == "POST":
		var v Config
		if e = decode(w, r, &v); e != nil {
			return e
		}
		t, e := v.Expanded()
		if e != nil {
			return invalid(e.Error())
		}
		reply(w, 200, t)
	case path == "/config" && r.Method == "GET":
		targets, e := c.Expanded()
		if e != nil {
			return e
		}
		reply(w, 200, map[string]any{"targets": targets, "flagFormat": c.FlagFormat, "interval": c.ClientInterval, "workers": c.ClientWorkers, "timeout": c.Timeout})
	case path == "/client.py" && r.Method == "GET":
		w.Header().Set("Content-Type", "text/x-python")
		w.Header().Set("Content-Disposition", `attachment; filename="gomate_client.py"`)
		_, e = w.Write([]byte(clientPython))
		return e
	case path == "/flags" && r.Method == "POST":
		var v struct {
			Text   string   `json:"text"`
			Team   string   `json:"team"`
			Sploit string   `json:"sploit"`
			Flags  []string `json:"flags"`
		}
		if e = decode(w, r, &v); e != nil {
			return e
		}
		if v.Team == "" {
			v.Team = "*"
		}
		if v.Sploit == "" {
			v.Sploit = "Manual"
		}
		if len(v.Team) > 128 || len(v.Sploit) > 128 || len(v.Flags) > 1000 {
			return invalid("Too many flags or metadata too long")
		}
		re := regexp.MustCompile(c.FlagFormat)
		if v.Text != "" {
			v.Flags = append(v.Flags, re.FindAllString(v.Text, 1001)...)
		}
		if len(v.Flags) > 1000 {
			return invalid("At most 1000 flags per request")
		}
		if len(v.Flags) == 0 {
			return invalid("No flags matching the configured format")
		}
		for _, f := range v.Flags {
			if len(f) > 4096 || re.FindString(f) != f || f == "" {
				return invalid("Flag does not match configured format")
			}
		}
		tx, e := s.pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(748351209)`); e != nil {
			return e
		}
		var pending int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM gomate_farm_flags WHERE status IN ('QUEUED','SUBMITTING')`).Scan(&pending); e != nil {
			return e
		}
		if pending+len(v.Flags) > 100000 {
			return invalid("Farm queue is full; enable submission or wait for flag expiry")
		}
		inserted := int64(0)
		for _, f := range v.Flags {
			tag, e := tx.Exec(ctx, `INSERT INTO gomate_farm_flags(flag,team,sploit) VALUES($1,$2,$3) ON CONFLICT(flag) DO NOTHING`, f, v.Team, v.Sploit)
			if e != nil {
				return e
			}
			inserted += tag.RowsAffected()
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		reply(w, 200, map[string]int64{"inserted": inserted, "duplicates": int64(len(v.Flags)) - inserted})
	case path == "/flags" && r.Method == "GET":
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		if page > 10000 {
			return invalid("Page limit exceeded; narrow filters")
		}
		status, team, sploit, search := q.Get("status"), q.Get("team"), q.Get("sploit"), q.Get("search")
		var since, until any
		for key, dest := range map[string]*any{"since": &since, "until": &until} {
			if v := q.Get(key); v != "" {
				t, e := time.Parse(time.RFC3339, v)
				if e != nil {
					return invalid("Invalid time filter")
				}
				*dest = t
			}
		}
		args := []any{status, team, sploit, search, since, until}
		where := ` WHERE ($1='' OR status=$1) AND ($2='' OR team=$2) AND ($3='' OR sploit=$3) AND ($4='' OR strpos(lower(flag||' '||response),lower($4))>0) AND ($5::timestamptz IS NULL OR created_at >=$5) AND ($6::timestamptz IS NULL OR created_at <=$6)`
		var count int64
		if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM gomate_farm_flags`+where, args...).Scan(&count); e != nil {
			return e
		}
		args = append(args, (page-1)*50)
		rows, e := s.pool.Query(ctx, `SELECT id,flag,team,sploit,status,response,attempts,created_at,updated_at FROM gomate_farm_flags`+where+` ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $7`, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		flags := []Flag{}
		for rows.Next() {
			var f Flag
			if e = rows.Scan(&f.ID, &f.Flag, &f.Team, &f.Sploit, &f.Status, &f.Response, &f.Attempts, &f.Created, &f.Updated); e != nil {
				return e
			}
			flags = append(flags, f)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		reply(w, 200, map[string]any{"flags": flags, "total": count, "page": page, "pageSize": 50})
	case path == "/summary" && r.Method == "GET":
		rows, e := s.pool.Query(ctx, `SELECT team,sploit,status,count(*) FROM gomate_farm_flags GROUP BY team,sploit,status ORDER BY team,sploit LIMIT 10000`)
		if e != nil {
			return e
		}
		defer rows.Close()
		type row struct {
			Team   string `json:"team"`
			Sploit string `json:"sploit"`
			Status string `json:"status"`
			Count  int64  `json:"count"`
		}
		out := []row{}
		for rows.Next() {
			var v row
			if e = rows.Scan(&v.Team, &v.Sploit, &v.Status, &v.Count); e != nil {
				return e
			}
			out = append(out, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		last, at := s.state()
		reply(w, 200, map[string]any{"groups": out, "enabled": c.Enabled, "protocol": c.Protocol, "lastError": last, "lastRun": at})
	default:
		http.NotFound(w, r)
	}
	return nil
}
