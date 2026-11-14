package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"gomate/internal/analyze"
	"gomate/internal/model"
)

type Job struct {
	ID        int64         `json:"id"`
	Pattern   model.Pattern `json:"pattern"`
	Status    string        `json:"status"`
	Processed int64         `json:"processedPackets"`
	Matched   int64         `json:"matchedPackets"`
	Cursor    int64         `json:"streamCursor"`
	MaxStream int64         `json:"maxStream"`
	Created   time.Time     `json:"createdAt"`
	Updated   time.Time     `json:"updatedAt"`
}

func (s *Store) StartLookback(ctx context.Context, p model.Pattern, since time.Time) (int64, error) {
	p.Enabled = true
	p.Deleted = false
	b, e := json.Marshal(p)
	if e != nil {
		return 0, e
	}
	var id int64
	e = s.Pool.QueryRow(ctx, `INSERT INTO gomate_jobs(pattern,since_ms,max_stream,max_packet) VALUES($1,$2,(SELECT COALESCE(max(id),0) FROM gomate_streams),(SELECT COALESCE(max(id),0) FROM gomate_packets)) RETURNING id`, b, since.UnixMilli()).Scan(&id)
	return id, e
}
func (s *Store) Jobs(ctx context.Context) ([]Job, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id,pattern,status,processed,matched,stream_cursor,max_stream,created_at,updated_at FROM gomate_jobs ORDER BY id DESC LIMIT 20`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		var b []byte
		if e = rows.Scan(&j.ID, &b, &j.Status, &j.Processed, &j.Matched, &j.Cursor, &j.MaxStream, &j.Created, &j.Updated); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &j.Pattern); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (s *Store) CancelLookback(ctx context.Context, id int64) error {
	r, e := s.Pool.Exec(ctx, `UPDATE gomate_jobs SET status='cancelled',updated_at=now() WHERE id=$1 AND status IN ('queued','running')`, id)
	if e == nil && r.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}

// One bounded transaction persists matches AND the cursor/tails. Process restart
// resumes exactly there. Live append-only inserts never have their matches erased.
func (s *Store) StepLookback(ctx context.Context) (bool, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer rollback(tx)
	var id, since, maxStream, maxPacket, sc, pc int64
	var pattern, state []byte
	e = tx.QueryRow(ctx, `SELECT id,pattern,since_ms,max_stream,max_packet,stream_cursor,packet_cursor,match_state FROM gomate_jobs WHERE status IN ('queued','running') ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &pattern, &since, &maxStream, &maxPacket, &sc, &pc, &state)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var p model.Pattern
	var ms analyze.MatchState
	if e = json.Unmarshal(pattern, &p); e != nil {
		return false, e
	}
	if e = json.Unmarshal(state, &ms); e != nil {
		return false, e
	}
	cat := analyze.NewCatalog()
	if e = cat.Replace(nil, []model.Pattern{p}); e != nil {
		return false, e
	}
	var stream model.Stream
	if pc == 0 {
		e = tx.QueryRow(ctx, `SELECT id,service,protocol FROM gomate_streams WHERE id>$1 AND id<=$2 AND end_ms>=$3 AND NOT ignored AND ($4::integer IS NULL OR service=$4) ORDER BY id LIMIT 1`, sc, maxStream, since, p.ServiceID).Scan(&stream.ID, &stream.Service, &stream.Protocol)
	} else {
		e = tx.QueryRow(ctx, `SELECT id,service,protocol FROM gomate_streams WHERE id=$1`, sc).Scan(&stream.ID, &stream.Service, &stream.Protocol)
	}
	if errors.Is(e, pgx.ErrNoRows) {
		if pc == 0 {
			_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET status='completed',updated_at=now() WHERE id=$1`, id)
		} else {
			_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET packet_cursor=0,match_state='{}',updated_at=now() WHERE id=$1`, id)
		}
		if e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	if e != nil {
		return false, e
	}
	rows, e := tx.Query(ctx, `SELECT id,timestamp_ms,incoming,content,incomplete,details FROM gomate_packets WHERE stream_id=$1 AND id>$2 AND id<=$3 ORDER BY id LIMIT 8`, stream.ID, pc, maxPacket)
	if e != nil {
		return false, e
	}
	packets := []model.Packet{}
	for rows.Next() {
		var q model.Packet
		var details []byte
		if e = rows.Scan(&q.ID, &q.Timestamp, &q.Incoming, &q.Content, &q.Incomplete, &details); e != nil {
			rows.Close()
			return false, e
		}
		var extra model.Packet
		if e = json.Unmarshal(details, &extra); e != nil {
			rows.Close()
			return false, e
		}
		q.Kind = extra.Kind
		packets = append(packets, q)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return false, e
	}
	if len(packets) == 0 {
		_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET stream_cursor=$2,packet_cursor=0,match_state='{}',status='running',updated_at=now() WHERE id=$1`, id, stream.ID)
		if e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	c := model.Chunk{Stream: stream, Packets: packets}
	cat.AnalyzeStreaming(&c, &ms)
	matched := 0
	for _, q := range c.Packets {
		if len(q.Matches) == 0 {
			continue
		}
		matched++
		b, e := json.Marshal(q.Matches)
		if e != nil {
			return false, e
		}
		_, e = tx.Exec(ctx, `UPDATE gomate_packets SET matches=COALESCE((SELECT jsonb_agg(m) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(matches)='array' THEN matches ELSE '[]'::jsonb END) m WHERE (m->>'patternId')::integer<>$2),'[]'::jsonb)||$3::jsonb WHERE id=$1`, q.ID, p.ID, b)
		if e != nil {
			return false, e
		}
	}
	if matched > 0 || c.Ignore {
		_, e = tx.Exec(ctx, `UPDATE gomate_streams SET pattern_ids=CASE WHEN $3 THEN ARRAY(SELECT DISTINCT unnest(pattern_ids||ARRAY[$2]::integer[])) ELSE pattern_ids END,ignored=ignored OR $4 WHERE id=$1`, stream.ID, p.ID, matched > 0, c.Ignore)
		if e != nil {
			return false, e
		}
	}
	b, e := json.Marshal(ms)
	if e != nil {
		return false, e
	}
	_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET status='running',stream_cursor=$2,packet_cursor=$3,match_state=$4,processed=processed+$5,matched=matched+$6,updated_at=now() WHERE id=$1`, id, stream.ID, packets[len(packets)-1].ID, b, len(packets), matched)
	if e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
