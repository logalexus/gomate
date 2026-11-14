package storage

import (
	"context"
	"encoding/json"

	"gomate/internal/anomaly"
	"gomate/internal/model"
)

// StepBehaviorBackfill reads at most 25 * 8 bounded packet prefixes. It is
// resumable, skips deleted sessions and never overwrites concurrent live writes.
func (s *Store) StepBehaviorBackfill(ctx context.Context) (bool, error) {
	rows, err := s.Pool.Query(ctx, `WITH candidates AS (
 SELECT s.id, COALESCE((v.data->>'http')::boolean,false) OR COALESCE((v.data->>'parseWebSockets')::boolean,false)
 OR COALESCE((v.data->>'urldecodeHttpRequests')::boolean,false)
 OR EXISTS(SELECT 1 FROM gomate_packets p WHERE p.stream_id=s.id AND COALESCE(p.details->>'kind','') IN ('http','http2-headers','grpc','websocket')) AS decoded
 FROM gomate_streams s LEFT JOIN gomate_services v ON v.port=s.service
 WHERE s.behavior IS NULL AND NOT s.ignored ORDER BY s.id LIMIT 25)
 SELECT c.id,p.details,substring(p.content FROM 1 FOR 65537)
 FROM candidates c LEFT JOIN LATERAL (
 SELECT id,details,content FROM gomate_packets p WHERE p.stream_id=c.id AND p.incoming
 AND (COALESCE(p.details->>'kind','') IN ('http','http2-headers','http2-body','grpc','websocket')
 OR (NOT c.decoded AND COALESCE(p.details->>'kind','')=''))
 ORDER BY p.id LIMIT 8) p ON true ORDER BY c.id,p.id`)
	if err != nil {
		return false, err
	}
	descriptors := map[int64][]anomaly.BehaviorMessage{}
	for rows.Next() {
		var id int64
		var details, content []byte
		if err = rows.Scan(&id, &details, &content); err != nil {
			rows.Close()
			return false, err
		}
		if _, ok := descriptors[id]; !ok {
			descriptors[id] = []anomaly.BehaviorMessage{}
		}
		if len(content) == 0 {
			continue
		}
		var p model.Packet
		if err = json.Unmarshal(details, &p); err != nil {
			rows.Close()
			return false, err
		}
		p.Content = content
		p.Incoming = true
		if b, ok := anomaly.BehaviorPacket(p); ok {
			descriptors[id] = append(descriptors[id], b)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	if len(descriptors) == 0 {
		return false, nil
	}
	values := []map[string]any{}
	for id, behavior := range descriptors {
		values = append(values, map[string]any{"id": id, "behavior": behavior})
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return false, err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE gomate_streams s SET behavior=x.behavior FROM jsonb_to_recordset($1::jsonb) AS x(id bigint,behavior jsonb) WHERE s.id=x.id AND s.behavior IS NULL`, payload)
	return err == nil, err
}
