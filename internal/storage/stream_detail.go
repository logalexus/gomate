package storage

import (
	"context"
	"gomate/internal/model"
)

// Stream loads connection metadata once when opening the request workbench.
func (s *Store) Stream(ctx context.Context, id int64) (model.Stream, error) {
	var v model.Stream
	err := s.Pool.QueryRow(ctx, `SELECT id,service,protocol,start_ms,end_ms,pattern_ids,favorite,ttl,size_bytes,packets_count,incomplete,client,server,user_agent_hash,payload_purged,
 COALESCE(first_request_path,''),COALESCE(first_request_method,''),COALESCE(first_request_truncated,false)
 FROM gomate_streams WHERE id=$1 AND NOT ignored`, id).Scan(&v.ID, &v.Service, &v.Protocol, &v.Start, &v.End, &v.Patterns, &v.Favorite, &v.TTL, &v.Size, &v.PacketsCount, &v.Incomplete, &v.Client, &v.Server, &v.UserAgentHash, &v.PayloadPurged, &v.FirstRequestPath, &v.FirstRequestMethod, &v.FirstRequestTruncated)
	return v, err
}
