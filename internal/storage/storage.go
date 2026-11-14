package storage

import (
	"context"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gomate/internal/anomaly"
	"gomate/internal/model"
)

//go:embed schema.sql
var schema string

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	c, e := pgxpool.ParseConfig(url)
	if e != nil {
		return nil, e
	}
	c.MaxConns = 8
	c.MinConns = 0
	c.ConnConfig.RuntimeParams["statement_timeout"] = "3000"
	c.ConnConfig.RuntimeParams["lock_timeout"] = "1000"
	c.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "5000"
	p, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	s := &Store{p}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(83472831)"); e == nil {
		_, e = tx.Exec(ctx, schema)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		p.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() { s.Pool.Close() }
func (s *Store) Services(ctx context.Context) ([]model.Service, error) {
	rows, e := s.Pool.Query(ctx, "SELECT data FROM gomate_services ORDER BY port")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Service{}
	for rows.Next() {
		var b []byte
		var v model.Service
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) PutService(ctx context.Context, v model.Service) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.Pool.Exec(ctx, "INSERT INTO gomate_services(port,data) VALUES($1,$2) ON CONFLICT(port) DO UPDATE SET data=excluded.data", v.Port, b)
	return e
}
func (s *Store) DeleteService(ctx context.Context, port int) error {
	_, e := s.Pool.Exec(ctx, "DELETE FROM gomate_services WHERE port=$1", port)
	return e
}
func (s *Store) Patterns(ctx context.Context) ([]model.Pattern, error) {
	rows, e := s.Pool.Query(ctx, "SELECT id,data FROM gomate_patterns ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Pattern{}
	for rows.Next() {
		var id int
		var b []byte
		var v model.Pattern
		if e = rows.Scan(&id, &b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.ID = id
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) PutPattern(ctx context.Context, p *model.Pattern) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if p.ID == 0 {
		return s.Pool.QueryRow(ctx, "INSERT INTO gomate_patterns(data) VALUES($1) RETURNING id", b).Scan(&p.ID)
	}
	r, e := s.Pool.Exec(ctx, "UPDATE gomate_patterns SET data=$1 WHERE id=$2", b, p.ID)
	if e == nil && r.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}

// SaveChunks commits receipt IDs, stream metadata and payload atomically.
// A retry following an ambiguous commit inserts no duplicate payload.
func (s *Store) SaveChunks(ctx context.Context, chunks []*model.Chunk) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tokens := make([]string, len(chunks))
	for i, c := range chunks {
		tokens[i] = c.Token
	}
	receipts, err := tx.Query(ctx, `INSERT INTO gomate_chunks(token) SELECT unnest($1::text[]) ON CONFLICT DO NOTHING RETURNING token`, tokens)
	if err != nil {
		return err
	}
	freshTokens := make(map[string]bool, len(chunks))
	for receipts.Next() {
		var token string
		if err = receipts.Scan(&token); err != nil {
			receipts.Close()
			return err
		}
		freshTokens[token] = true
	}
	receipts.Close()
	if err = receipts.Err(); err != nil {
		return err
	}
	fresh := make([]*model.Chunk, 0, len(chunks))
	type aggregate struct {
		stream   model.Stream
		bytes    int64
		packets  int
		ignore   bool
		patterns map[int]bool
		behavior []anomaly.BehaviorMessage
	}
	groups := make(map[string]*aggregate)
	keys := []string{}
	for _, c := range chunks {
		if !freshTokens[c.Token] {
			continue
		}
		delete(freshTokens, c.Token)
		fresh = append(fresh, c)
		a := groups[c.Stream.Key]
		if a == nil {
			a = &aggregate{stream: c.Stream, patterns: map[int]bool{}}
			groups[c.Stream.Key] = a
			keys = append(keys, c.Stream.Key)
		}
		if c.Stream.UserAgentHash != nil {
			a.stream.UserAgentHash = c.Stream.UserAgentHash
		}
		if a.stream.FirstRequestPath == "" {
			for _, packet := range c.Packets {
				if !packet.Incoming || (packet.Kind != "http" && packet.Kind != "http2-headers" && packet.Kind != "grpc") || packet.Metadata["path"] == "" {
					continue
				}
				var methodTruncated bool
				a.stream.FirstRequestPath, a.stream.FirstRequestTruncated = boundedRequestField(packet.Metadata["path"], 2048)
				a.stream.FirstRequestMethod, methodTruncated = boundedRequestField(packet.Metadata["method"], 32)
				a.stream.FirstRequestTruncated = a.stream.FirstRequestTruncated || methodTruncated
				break
			}
		}
		for _, packet := range c.Packets {
			if c.Ignore || (c.Decoded && packet.Kind == "") {
				continue
			}
			if len(a.behavior) >= anomaly.BehaviorMessages {
				break
			}
			if b, ok := anomaly.BehaviorPacket(packet); ok {
				a.behavior = append(a.behavior, b)
			}
		}
		a.bytes += c.Bytes
		a.packets += len(c.Packets)
		a.ignore = a.ignore || c.Ignore
		a.stream.Incomplete = a.stream.Incomplete || c.Stream.Incomplete
		if c.Stream.End > a.stream.End {
			a.stream.End = c.Stream.End
		}
		for _, id := range c.Stream.Patterns {
			a.patterns[id] = true
		}
	}
	if len(fresh) == 0 {
		return tx.Commit(ctx)
	}
	metadata := make([]map[string]any, 0, len(groups))
	for _, key := range keys {
		a := groups[key]
		v := a.stream
		ids := []int{}
		for id := range a.patterns {
			ids = append(ids, id)
		}
		metadata = append(metadata, map[string]any{"flow_key": key, "service": v.Service, "protocol": v.Protocol, "start_ms": v.Start, "end_ms": v.End, "pattern_ids": ids, "incomplete": v.Incomplete, "ignored": a.ignore, "ttl": v.TTL, "size_bytes": a.bytes, "packets_count": a.packets, "client": v.Client, "server": v.Server, "user_agent_hash": v.UserAgentHash, "first_request_path": v.FirstRequestPath, "first_request_method": v.FirstRequestMethod, "first_request_truncated": v.FirstRequestTruncated, "behavior": a.behavior})
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	records, err := tx.Query(ctx, `INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,pattern_ids,incomplete,ignored,ttl,size_bytes,packets_count,client,server,user_agent_hash,first_request_path,first_request_method,first_request_truncated,behavior)
    SELECT flow_key,service,protocol,start_ms,end_ms,pattern_ids,incomplete,ignored,ttl,size_bytes,packets_count,client,server,user_agent_hash,NULLIF(first_request_path,''),NULLIF(first_request_method,''),first_request_truncated,COALESCE(NULLIF(behavior,'null'::jsonb),'[]'::jsonb)
    FROM jsonb_to_recordset($1::jsonb) AS x(flow_key text,service integer,protocol text,start_ms bigint,end_ms bigint,pattern_ids integer[],incomplete boolean,ignored boolean,ttl integer,size_bytes bigint,packets_count integer,client text,server text,user_agent_hash text,first_request_path text,first_request_method text,first_request_truncated boolean,behavior jsonb)
    ON CONFLICT(flow_key) DO UPDATE SET end_ms=GREATEST(gomate_streams.end_ms,excluded.end_ms),
    pattern_ids=ARRAY(SELECT DISTINCT unnest(gomate_streams.pattern_ids||excluded.pattern_ids)),
    user_agent_hash=COALESCE(excluded.user_agent_hash,gomate_streams.user_agent_hash),
    first_request_path=COALESCE(gomate_streams.first_request_path,excluded.first_request_path),
    first_request_method=CASE WHEN gomate_streams.first_request_path IS NULL THEN excluded.first_request_method ELSE gomate_streams.first_request_method END,
    first_request_truncated=CASE WHEN gomate_streams.first_request_path IS NULL THEN excluded.first_request_truncated ELSE gomate_streams.first_request_truncated END,
 behavior=jsonb_path_query_array(COALESCE(gomate_streams.behavior,'[]'::jsonb)||excluded.behavior,'$[0 to 7]'),
 incomplete=gomate_streams.incomplete OR excluded.incomplete,ignored=gomate_streams.ignored OR excluded.ignored,
    size_bytes=gomate_streams.size_bytes+excluded.size_bytes,packets_count=gomate_streams.packets_count+excluded.packets_count
    RETURNING flow_key,id,ignored`, payload)
	if err != nil {
		return err
	}
	type saved struct {
		id      int64
		ignored bool
	}
	ids := map[string]saved{}
	for records.Next() {
		var key string
		var v saved
		if err = records.Scan(&key, &v.id, &v.ignored); err != nil {
			records.Close()
			return err
		}
		ids[key] = v
	}
	records.Close()
	if err = records.Err(); err != nil {
		return err
	}
	rows := make([][]any, 0)
	for _, c := range fresh {
		v := ids[c.Stream.Key]
		if v.ignored {
			continue
		}
		c.Stream.ID = v.id
		for _, p := range c.Packets {
			if p.Content == nil {
				p.Content = []byte{}
			}
			b, e := json.Marshal(p.Matches)
			if e != nil {
				return e
			}
			detail := p
			detail.Content = nil
			detail.Matches = nil
			detail.ID = 0
			extra, e := json.Marshal(detail)
			if e != nil {
				return e
			}
			rows = append(rows, []any{v.id, p.Timestamp, p.Incoming, p.Content, b, p.Incomplete, extra})
		}
	}
	if len(rows) > 0 {
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"gomate_packets"}, []string{"stream_id", "timestamp_ms", "incoming", "content", "matches", "incomplete", "details"}, pgx.CopyFromRows(rows))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Streams(ctx context.Context, port int, page model.Page) ([]model.Stream, error) {
	patterns, e := page.SelectedPatternIDs()
	if e != nil {
		return nil, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,service,protocol,start_ms,end_ms,pattern_ids,favorite,ttl,size_bytes,packets_count,incomplete,client,server,user_agent_hash,payload_purged,
 COALESCE(first_request_path,''),COALESCE(first_request_method,''),COALESCE(first_request_truncated,false)
 FROM gomate_streams WHERE NOT ignored AND ($1=0 OR service=$1) AND ($2::bigint IS NULL OR id<$2) AND (NOT $3 OR favorite) AND pattern_ids @> $4::integer[] ORDER BY id DESC LIMIT $5`, port, page.StartingFrom, page.Favorites, patterns, page.PageSize)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Stream{}
	for rows.Next() {
		var v model.Stream
		if e = rows.Scan(&v.ID, &v.Service, &v.Protocol, &v.Start, &v.End, &v.Patterns, &v.Favorite, &v.TTL, &v.Size, &v.PacketsCount, &v.Incomplete, &v.Client, &v.Server, &v.UserAgentHash, &v.PayloadPurged, &v.FirstRequestPath, &v.FirstRequestMethod, &v.FirstRequestTruncated); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) PacketsPage(ctx context.Context, id int64, page model.Page) ([]model.Packet, bool, error) {
	rows, e := s.Pool.Query(ctx, `SELECT p.id,p.timestamp_ms,p.incoming,p.content,p.matches,p.incomplete,p.details FROM gomate_packets p JOIN gomate_streams s ON s.id=p.stream_id WHERE NOT s.ignored AND stream_id=$1 AND ($2::bigint IS NULL OR p.id>$2) AND ($4='' OR ($4='raw' AND COALESCE(p.details->>'kind','')='') OR ($4='decoded' AND COALESCE(p.details->>'kind','')<>'')) ORDER BY p.id LIMIT $3`, id, page.StartingFrom, page.PageSize+1, page.Representation)
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	out := []model.Packet{}
	size := 0
	for rows.Next() {
		var p model.Packet
		var b, details []byte
		if e = rows.Scan(&p.ID, &p.Timestamp, &p.Incoming, &p.Content, &b, &p.Incomplete, &details); e != nil {
			return nil, false, e
		}
		var extra model.Packet
		if e = json.Unmarshal(details, &extra); e != nil {
			return nil, false, e
		}
		p.Kind = extra.Kind
		p.Metadata = extra.Metadata
		p.Ungzipped = extra.Ungzipped
		p.WebSocketParsed = extra.WebSocketParsed
		p.TLSDecrypted = extra.TLSDecrypted
		p.HasHTTPBody = extra.HasHTTPBody
		if e = json.Unmarshal(b, &p.Matches); e != nil {
			return nil, false, e
		}
		size += len(p.Content)
		if len(out) >= page.PageSize || size > 4<<20 && len(out) > 0 {
			return out, true, nil
		}
		out = append(out, p)
	}
	return out, false, rows.Err()
}
func (s *Store) Packets(ctx context.Context, id int64, page model.Page) ([]model.Packet, error) {
	out, _, e := s.PacketsPage(ctx, id, page)
	return out, e
}

func (s *Store) Favorite(ctx context.Context, id int64, v bool) error {
	r, e := s.Pool.Exec(ctx, "UPDATE gomate_streams SET favorite=$2 WHERE id=$1 AND NOT ignored", id, v)
	if e == nil && r.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}
func (s *Store) Cleanup(ctx context.Context, before time.Time) error {
	// Each transaction is small. A wall-time budget limits interference with
	// capture, while repeated batches avoid a fixed 100-rows/min retention cap.
	for i := 0; i < 100; i++ {
		r, e := s.Pool.Exec(ctx, `DELETE FROM gomate_streams WHERE id IN (SELECT id FROM gomate_streams WHERE NOT favorite AND end_ms<$1 ORDER BY end_ms LIMIT 100)`, before.UnixMilli())
		if e != nil {
			return e
		}
		if r.RowsAffected() < 100 {
			break
		}
	}
	// One hour exceeds the writer's bounded retry window by a wide margin.
	_, e := s.Pool.Exec(ctx, `DELETE FROM gomate_chunks WHERE token IN (SELECT token FROM gomate_chunks WHERE created_at<now()-interval '1 hour' LIMIT 10000)`)
	return e
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
