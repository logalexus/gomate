package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"gomate/internal/analyze"
	"gomate/internal/model"
	"strings"
	"time"
)

// ImportLegacy operates on an offline, fixed source snapshot. It never executes
// source mutations. Destination IDs remain stable, preserving favorites/matches.
// Checkpoints commit with each bounded batch; rerunning resumes after interruption.
func (s *Store) ImportLegacy(ctx context.Context, sourceURL, sourceID string, history bool) error {
	cfg, e := pgx.ParseConfig(sourceURL)
	if e != nil {
		return e
	}
	cfg.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.RuntimeParams["statement_timeout"] = "5000"
	src, e := pgx.ConnectConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer src.Close(context.Background())
	// Serialize imports and refuse a destination already containing live data.
	conn, e := s.Pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(83472832)").Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return fmt.Errorf("another import is active")
	}
	defer func() {
		q, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		conn.Exec(q, "SELECT pg_advisory_unlock(83472832)")
	}()
	var services []model.Service
	rows, e := src.Query(ctx, `SELECT port,name,http,urldecode_http_requests,merge_adjacent_packets,parse_web_sockets,decrypt_tls FROM service ORDER BY port`)
	if e != nil {
		return fmt.Errorf("legacy service schema: %w", e)
	}
	for rows.Next() {
		var v model.Service
		if e = rows.Scan(&v.Port, &v.Name, &v.HTTP, &v.URLDecode, &v.Merge, &v.WebSockets, &v.TLS); e != nil {
			rows.Close()
			return e
		}
		if v.TLS {
			rows.Close()
			return fmt.Errorf("service %d requires legacy TLS decryption; import a copy with decrypt_tls disabled after reviewing this limitation", v.Port)
		}
		services = append(services, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	patterns := []model.Pattern{}
	rows, e = src.Query(ctx, `SELECT id,enabled,deleted,name,value,color,search_type,direction_type,action_type,service_id FROM pattern ORDER BY id`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var p model.Pattern
		var st, dt, at int
		if e = rows.Scan(&p.ID, &p.Enabled, &p.Deleted, &p.Name, &p.Value, &p.Color, &st, &dt, &at, &p.ServiceID); e != nil {
			rows.Close()
			return e
		}
		if st < 0 || st > 2 || dt < 0 || dt > 2 || at < 0 || at > 1 {
			rows.Close()
			return fmt.Errorf("invalid legacy enum for pattern %d", p.ID)
		}
		p.SearchType = []string{"REGEX", "SUBSTRING", "SUBBYTES"}[st]
		p.Direction = []string{"INPUT", "OUTPUT", "BOTH"}[dt]
		p.Action = []string{"FIND", "IGNORE"}[at]
		if p.Enabled && !p.Deleted {
			if e = analyze.Validate(p); e != nil {
				rows.Close()
				return fmt.Errorf("pattern %d is not compatible with Go/RE2: %w", p.ID, e)
			}
		}
		patterns = append(patterns, p)
		if len(patterns) > 10000 {
			rows.Close()
			return fmt.Errorf("legacy pattern count exceeds import budget")
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(tx)
	var storedID, phase string
	var sc, pc int64
	e = tx.QueryRow(ctx, `SELECT source_id,phase,stream_cursor,packet_cursor FROM gomate_import_state WHERE singleton`).Scan(&storedID, &phase, &sc, &pc)
	if e == pgx.ErrNoRows {
		var count int
		if e = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM gomate_streams)+(SELECT count(*) FROM gomate_patterns)+(SELECT count(*) FROM gomate_services)`).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			return fmt.Errorf("first import requires an empty destination; stop gomate and use a separate database")
		}
		if _, e = tx.Exec(ctx, `INSERT INTO gomate_import_state(source_id) VALUES($1)`, sourceID); e != nil {
			return e
		}
		phase = "settings"
	} else if e != nil {
		return e
	} else if storedID != sourceID {
		return fmt.Errorf("destination belongs to another source snapshot")
	}
	if phase == "settings" {
		for _, v := range services {
			b, _ := json.Marshal(v)
			if _, e = tx.Exec(ctx, `INSERT INTO gomate_services(port,data) VALUES($1,$2) ON CONFLICT DO NOTHING`, v.Port, b); e != nil {
				return e
			}
		}
		for _, p := range patterns {
			b, _ := json.Marshal(p)
			if _, e = tx.Exec(ctx, `INSERT INTO gomate_patterns(id,data) OVERRIDING SYSTEM VALUE VALUES($1,$2) ON CONFLICT DO NOTHING`, p.ID, b); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE gomate_import_state SET phase='streams'`); e != nil {
			return e
		}
	}
	if e = resetSequences(ctx, tx); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if !history {
		return nil
	}
	for phase != "packets" && phase != "complete" {
		rows, e = src.Query(ctx, `SELECT id,service_id,protocol,start_timestamp,end_timestamp,favorite,ttl,user_agent_hash,size_bytes,packets_count,ARRAY(SELECT pattern_id FROM stream_found_patterns fp WHERE fp.stream_id=stream.id) FROM stream WHERE id>$1 ORDER BY id LIMIT 100`, sc)
		if e != nil {
			return e
		}
		batch := []model.Stream{}
		for rows.Next() {
			var v model.Stream
			var proto int
			if e = rows.Scan(&v.ID, &v.Service, &proto, &v.Start, &v.End, &v.Favorite, &v.TTL, &v.UserAgentHash, &v.Size, &v.PacketsCount, &v.Patterns); e != nil {
				rows.Close()
				return e
			}
			if proto < 0 || proto > 1 {
				rows.Close()
				return fmt.Errorf("invalid legacy protocol")
			}
			v.Protocol = []string{"TCP", "UDP"}[proto]
			batch = append(batch, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		tx, e = s.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		for _, v := range batch {
			_, e = tx.Exec(ctx, `INSERT INTO gomate_streams(id,flow_key,service,protocol,start_ms,end_ms,pattern_ids,favorite,ttl,size_bytes,packets_count,client,server,user_agent_hash) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'legacy','legacy',$12) ON CONFLICT(id) DO NOTHING`, v.ID, sourceID+fmt.Sprint("/legacy/", v.ID), v.Service, v.Protocol, v.Start, v.End, v.Patterns, v.Favorite, v.TTL, v.Size, v.PacketsCount, v.UserAgentHash)
			if e != nil {
				rollback(tx)
				return e
			}
			sc = v.ID
		}
		phase = "streams"
		if len(batch) < 100 {
			phase = "packets"
		}
		_, e = tx.Exec(ctx, `UPDATE gomate_import_state SET phase=$1,stream_cursor=$2`, phase, sc)
		if e == nil {
			e = resetSequences(ctx, tx)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		rollback(tx)
		if e != nil {
			return e
		}
	}
	if phase != "complete" {
		// Refuse oversized legacy payloads before transferring them into process RAM.
		var large int64
		e = src.QueryRow(ctx, `SELECT id FROM packet WHERE id>$1 AND octet_length(content)>4194304 ORDER BY id LIMIT 1`, pc).Scan(&large)
		if e == nil {
			return fmt.Errorf("legacy packet %d exceeds 4 MiB import budget", large)
		}
		if e != pgx.ErrNoRows {
			return e
		}
	}
	for phase != "complete" {
		rows, e = src.Query(ctx, `SELECT id,stream_id,timestamp,incoming,content,http_processed,web_socket_parsed,tls_decrypted,has_http_body,COALESCE((SELECT jsonb_agg(jsonb_build_object('patternId',pattern_id,'startPosition',start_position,'endPosition',end_position)) FROM found_pattern f WHERE f.packet_id=packet.id),'[]'::jsonb) FROM packet WHERE id>$1 ORDER BY id LIMIT 8`, pc)
		if e != nil {
			return e
		}
		type record struct {
			p      model.Packet
			stream int64
		}
		batch := []record{}
		for rows.Next() {
			var v record
			var processed bool
			var matches []byte
			if e = rows.Scan(&v.p.ID, &v.stream, &v.p.Timestamp, &v.p.Incoming, &v.p.Content, &processed, &v.p.WebSocketParsed, &v.p.TLSDecrypted, &v.p.HasHTTPBody, &matches); e != nil {
				rows.Close()
				return e
			}
			v.p.Kind = "legacy"
			v.p.Metadata = map[string]string{"source": "Packmate Java", "httpProcessed": fmt.Sprint(processed)}
			if e = json.Unmarshal(matches, &v.p.Matches); e != nil {
				rows.Close()
				return e
			}
			batch = append(batch, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		tx, e = s.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		for _, v := range batch {
			q := v.p
			q.Content = nil
			q.Matches = nil
			details, _ := json.Marshal(q)
			matches, _ := json.Marshal(v.p.Matches)
			_, e = tx.Exec(ctx, `INSERT INTO gomate_packets(id,stream_id,timestamp_ms,incoming,content,matches,details) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, v.p.ID, v.stream, v.p.Timestamp, v.p.Incoming, v.p.Content, matches, details)
			if e != nil {
				rollback(tx)
				return e
			}
			pc = v.p.ID
		}
		phase = "packets"
		if len(batch) < 8 {
			phase = "complete"
		}
		_, e = tx.Exec(ctx, `UPDATE gomate_import_state SET phase=$1,packet_cursor=$2`, phase, pc)
		if e == nil {
			e = resetSequences(ctx, tx)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		rollback(tx)
		if e != nil {
			return e
		}
	}
	return nil
}
func resetSequences(ctx context.Context, tx pgx.Tx) error {
	for _, table := range []string{"gomate_patterns", "gomate_streams", "gomate_packets"} {
		sql := `SELECT setval(pg_get_serial_sequence('TABLE','id'),GREATEST(COALESCE((SELECT max(id) FROM TABLE),0),1),EXISTS(SELECT 1 FROM TABLE))`
		if _, e := tx.Exec(ctx, strings.ReplaceAll(sql, "TABLE", table)); e != nil {
			return e
		}
	}
	return nil
}
