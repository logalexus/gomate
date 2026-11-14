package storage

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"gomate/internal/model"
	"math"
	"time"
)

type CleanupJob struct {
	ID                int64     `json:"id"`
	Mode              string    `json:"mode"`
	PreserveFavorites bool      `json:"preserveFavorites"`
	Service           int       `json:"service"`
	Status            string    `json:"status"`
	Deleted           int64     `json:"deletedRecords"`
	Created           time.Time `json:"createdAt"`
}
type CleanupStats struct {
	Packets  int64 `json:"packets"`
	Sessions int64 `json:"sessions"`
	Bytes    int64 `json:"payloadBytes"`
}

func (s *Store) CleanupPreview(ctx context.Context, keep bool, service int) (CleanupStats, error) {
	var v CleanupStats
	e := s.Pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum(octet_length(p.content)),0),count(DISTINCT s.id) FROM gomate_packets p JOIN gomate_streams s ON s.id=p.stream_id WHERE (NOT $1 OR NOT s.favorite) AND ($2=0 OR s.service=$2)`, keep, service).Scan(&v.Packets, &v.Bytes, &v.Sessions)
	if e != nil {
		return v, e
	}
	e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM gomate_streams WHERE (NOT $1 OR NOT favorite) AND ($2=0 OR service=$2)`, keep, service).Scan(&v.Sessions)
	return v, e
}
func (s *Store) StartCleanup(ctx context.Context, mode string, keep bool, service int) (int64, error) {
	var id int64
	e := s.Pool.QueryRow(ctx, `INSERT INTO gomate_cleanup_jobs(mode,preserve_favorites,service,max_id,max_packet) VALUES($1,$2,$3,(SELECT COALESCE(max(id),0) FROM gomate_streams),(SELECT COALESCE(max(id),0) FROM gomate_packets)) RETURNING id`, mode, keep, service).Scan(&id)
	return id, e
}
func (s *Store) CleanupJobs(ctx context.Context) ([]CleanupJob, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id,mode,preserve_favorites,service,status,deleted,created_at FROM gomate_cleanup_jobs ORDER BY id DESC LIMIT 10`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []CleanupJob{}
	for rows.Next() {
		var v CleanupJob
		if e = rows.Scan(&v.ID, &v.Mode, &v.PreserveFavorites, &v.Service, &v.Status, &v.Deleted, &v.Created); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CancelCleanup(ctx context.Context, id int64) error {
	r, e := s.Pool.Exec(ctx, `UPDATE gomate_cleanup_jobs SET status='cancelled',updated_at=now() WHERE id=$1 AND status IN ('queued','running')`, id)
	if e == nil && r.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}

// Payload is removed in small portions even for a single enormous session.
// Locks on the parent prevent a concurrent favorite toggle from racing deletion.
func cleanupBatch(ctx context.Context, tx pgx.Tx, mode string, keep bool, service int, maxStream, maxPacket, cutoff int64) (int64, error) {
	var n int64
	e := tx.QueryRow(ctx, `WITH candidates AS (
 SELECT p.id,p.stream_id FROM gomate_packets p JOIN gomate_streams s ON s.id=p.stream_id
 WHERE p.id<=$1 AND s.id<=$2 AND (NOT $3 OR NOT s.favorite) AND ($4=0 OR s.service=$4)
 AND (($5='packets' AND p.timestamp_ms<$6) OR ($5='sessions' AND s.end_ms<$6))
 ORDER BY p.id LIMIT 500 FOR UPDATE OF s SKIP LOCKED
 ), removed AS (DELETE FROM gomate_packets p USING candidates c WHERE p.id=c.id RETURNING p.stream_id)
  , marked AS (UPDATE gomate_streams SET payload_purged=true WHERE id IN (SELECT stream_id FROM removed)) SELECT count(*) FROM removed`, maxPacket, maxStream, keep, service, mode, cutoff).Scan(&n)
	if e != nil {
		return 0, e
	}
	if mode == "sessions" {
		candidates, e := tx.Query(ctx, `SELECT s.id FROM gomate_streams s WHERE s.id<=$1 AND s.end_ms<$2 AND (NOT $3 OR NOT s.favorite) AND ($4=0 OR s.service=$4) AND NOT EXISTS(SELECT 1 FROM gomate_packets p WHERE p.stream_id=s.id) ORDER BY s.id LIMIT 500 FOR UPDATE SKIP LOCKED`, maxStream, cutoff, keep, service)
		if e != nil {
			return 0, e
		}
		ids := []int64{}
		for candidates.Next() {
			var id int64
			if e = candidates.Scan(&id); e != nil {
				candidates.Close()
				return 0, e
			}
			ids = append(ids, id)
		}
		candidates.Close()
		if e = candidates.Err(); e != nil {
			return 0, e
		}
		// A fresh statement snapshot after locking parents prevents deleting a
		// packet committed concurrently by the live writer before our lock.
		r, e := tx.Exec(ctx, `DELETE FROM gomate_streams s WHERE id=ANY($1::bigint[]) AND s.end_ms<$2 AND (NOT $3 OR NOT s.favorite) AND NOT EXISTS(SELECT 1 FROM gomate_packets p WHERE p.stream_id=s.id)`, ids, cutoff, keep)
		if e != nil {
			return 0, e
		}
		n += r.RowsAffected()
	}

	return n, nil
}
func (s *Store) StepCleanup(ctx context.Context) (bool, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer rollback(tx)
	var id, ms, mp int64
	var mode string
	var keep bool
	var service int
	e = tx.QueryRow(ctx, `SELECT id,mode,preserve_favorites,service,max_id,max_packet FROM gomate_cleanup_jobs WHERE status IN ('queued','running') ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &mode, &keep, &service, &ms, &mp)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	n, e := cleanupBatch(ctx, tx, mode, keep, service, ms, mp, math.MaxInt64)
	if e != nil {
		return false, e
	}
	status := "running"
	if n == 0 {
		var pending bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM gomate_packets p JOIN gomate_streams s ON s.id=p.stream_id WHERE p.id<=$1 AND s.id<=$2 AND (NOT $3 OR NOT s.favorite) AND ($4=0 OR s.service=$4)) OR ($5='sessions' AND EXISTS(SELECT 1 FROM gomate_streams s WHERE s.id<=$2 AND (NOT $3 OR NOT s.favorite) AND ($4=0 OR s.service=$4) AND NOT EXISTS(SELECT 1 FROM gomate_packets p WHERE p.stream_id=s.id)))`, mp, ms, keep, service, mode).Scan(&pending)
		if e != nil {
			return false, e
		}
		if !pending {
			status = "completed"
		}
	}
	_, e = tx.Exec(ctx, `UPDATE gomate_cleanup_jobs SET status=$2,deleted=deleted+$3,updated_at=now() WHERE id=$1`, id, status, n)
	if e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
func (s *Store) Retain(ctx context.Context, v model.RuntimeSettings) error {
	for i := 0; i < 100; i++ {
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		var total int64
		for _, rule := range []struct {
			mode    string
			seconds int64
		}{{"packets", v.PacketRetentionSeconds}, {"sessions", v.SessionRetentionSeconds}} {
			if rule.seconds == 0 {
				continue
			}
			n, e := cleanupBatch(ctx, tx, rule.mode, v.PreserveFavorites, 0, math.MaxInt64, math.MaxInt64, time.Now().Add(-time.Duration(rule.seconds)*time.Second).UnixMilli())
			if e != nil {
				rollback(tx)
				return e
			}
			total += n
		}
		if i == 0 {
			_, e = tx.Exec(ctx, `DELETE FROM gomate_chunks WHERE token IN(SELECT token FROM gomate_chunks WHERE created_at<now()-interval '1 hour' LIMIT 1000)`)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		rollback(tx)
		if e != nil {
			return e
		}
		if total == 0 {
			break
		}
	}
	return nil
}
