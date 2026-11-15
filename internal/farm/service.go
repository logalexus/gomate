package farm

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

//go:embed schema.sql
var schema string
var ErrConflict = errors.New("configuration changed; reload settings")

type Service struct {
	pool      *pgxpool.Pool
	cancel    context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
	lastError string
	lastRun   time.Time
}

func New(ctx context.Context, p *pgxpool.Pool) (*Service, error) {
	init, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, e := p.Exec(init, schema); e != nil {
		return nil, e
	}
	b, _ := json.Marshal(Default())
	if _, e := p.Exec(init, `INSERT INTO gomate_farm_config(id,data) VALUES(true,$1) ON CONFLICT DO NOTHING`, b); e != nil {
		return nil, e
	}
	work, stop := context.WithCancel(ctx)
	s := &Service{pool: p, cancel: stop, done: make(chan struct{})}
	go s.run(work)
	return s, nil
}
func (s *Service) Close() { s.cancel(); <-s.done }
func (s *Service) Config(ctx context.Context) (Config, error) {
	var b []byte
	var rev int64
	e := s.pool.QueryRow(ctx, `SELECT revision,data FROM gomate_farm_config WHERE id`).Scan(&rev, &b)
	c := Default()
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	c.Revision = rev
	return c, e
}
func (s *Service) Save(ctx context.Context, c Config) (Config, error) {
	old, e := s.Config(ctx)
	if e != nil {
		return c, e
	}
	if c.Token == "" && !c.ClearToken {
		c.Token = old.Token
	}
	if c.ClearToken {
		c.Token = ""
	}
	c.ClearToken = false
	c.TokenSet = false
	if e = c.Validate(); e != nil {
		return c, e
	}
	b, _ := json.Marshal(c)
	tag, e := s.pool.Exec(ctx, `UPDATE gomate_farm_config SET revision=revision+1,data=$1 WHERE id AND revision=$2`, b, c.Revision)
	if e != nil {
		return c, e
	}
	if tag.RowsAffected() == 0 {
		return c, ErrConflict
	}
	c.Revision++
	return c, nil
}
func (s *Service) state() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastError, s.lastRun
}
func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		c, e := s.Config(ctx)
		delay := 2
		if e == nil {
			delay = c.Period
			e = s.step(ctx, c)
		}
		s.mu.Lock()
		s.lastRun = time.Now()
		s.lastError = ""
		if e != nil && ctx.Err() == nil {
			s.lastError = e.Error()
			slog.Warn("farm submission will retry", "error", e)
		}
		s.mu.Unlock()
		timer.Reset(time.Duration(delay) * time.Second)
	}
}

type Flag struct {
	ID       int64     `json:"id"`
	Flag     string    `json:"flag"`
	Team     string    `json:"team"`
	Sploit   string    `json:"sploit"`
	Status   string    `json:"status"`
	Response string    `json:"response"`
	Attempts int       `json:"attempts"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

func (s *Service) step(ctx context.Context, c Config) error {
	q, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Leases recover only after a full batch deadline, including after a process restart.
	if _, e := s.pool.Exec(q, `UPDATE gomate_farm_flags SET status='QUEUED',response='Recovered submission lease' WHERE status='SUBMITTING' AND next_at<now()`); e != nil {
		return e
	}
	if _, e := s.pool.Exec(q, `UPDATE gomate_farm_flags SET status='SKIPPED',response='Flag lifetime exceeded',updated_at=now() WHERE status='QUEUED' AND created_at<now()-make_interval(secs=>$1)`, c.Lifetime); e != nil {
		return e
	}
	if _, e := s.pool.Exec(q, `DELETE FROM gomate_farm_flags WHERE id IN (SELECT id FROM gomate_farm_flags WHERE status NOT IN ('QUEUED','SUBMITTING') AND created_at<now()-make_interval(secs=>$1) LIMIT 1000)`, c.Retention); e != nil {
		return e
	}
	if !c.Enabled {
		return nil
	}
	rows, e := s.pool.Query(q, `UPDATE gomate_farm_flags SET status='SUBMITTING',attempts=attempts+1,updated_at=now(),next_at=now()+make_interval(secs=>$2) WHERE id IN (SELECT id FROM gomate_farm_flags WHERE status='QUEUED' AND next_at<=now() ORDER BY attempts,id LIMIT $1 FOR UPDATE SKIP LOCKED) RETURNING id,flag,attempts`, c.Batch, c.Timeout*c.Batch+30)
	if e != nil {
		return e
	}
	flags := []Flag{}
	for rows.Next() {
		var f Flag
		if e = rows.Scan(&f.ID, &f.Flag, &f.Attempts); e != nil {
			rows.Close()
			return e
		}
		flags = append(flags, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(flags) == 0 {
		return e
	}
	client := &http.Client{Timeout: time.Duration(c.Timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	results := submit(ctx, client, c, flags)
	for i, f := range flags {
		r := results[i]
		backoff := c.Period * (1 << min(f.Attempts-1, 5))
		if backoff > 60 {
			backoff = 60
		}
		write, stop := context.WithTimeout(ctx, 5*time.Second)
		_, e = s.pool.Exec(write, `UPDATE gomate_farm_flags SET status=$2,response=$3,updated_at=now(),next_at=now()+make_interval(secs=>$4) WHERE id=$1 AND status='SUBMITTING'`, f.ID, r.Status, r.Response, backoff)
		stop()
		if e != nil {
			return e
		}
	}
	for _, r := range results {
		if r.Status == "QUEUED" {
			return fmt.Errorf("%s", r.Response)
		}
	}
	return nil
}
