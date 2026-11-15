package storage

import (
	"context"
	"regexp"
	"strings"
	"time"

	"gomate/internal/model"
)

// SearchTraffic reads a bounded slice of history using existing stream/packet
// indexes. It never runs a user regexp in PostgreSQL or loads entire sessions.
// The client follows Next, yielding between requests and allowing cancellation.
func (s *Store) SearchTraffic(ctx context.Context, search model.TrafficSearch, re *regexp.Regexp) (model.TrafficSearchResult, error) {
	out := model.TrafficSearchResult{Streams: []model.Stream{}}
	page := search.Page
	page.PageSize = 128
	page.StartingFrom = nil
	if search.Cursor != nil {
		page.StartingFrom = search.Cursor.Before
	}
	streams, err := s.Streams(ctx, search.Port, page)
	if err != nil {
		return out, err
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	packets, bytes := 0, 0
	for _, stream := range streams {
		id := stream.ID
		out.Next = &model.SearchCursor{Before: &id}
		after := int64(0)
		if search.Cursor != nil && search.Cursor.InStream == id {
			after = search.Cursor.AfterPacket
		}
		if after == 0 {
			out.Scanned++
		}
		// Metadata is searchable even after payload retention removes packets.
		matched := re.MatchString(strings.Join([]string{stream.FirstRequestMethod, stream.FirstRequestPath, stream.Client, stream.Server, stream.Protocol}, " "))
		if !matched {
			// Bound the actual result bytes, including data drained by rows.Close.
			// octet_length(bytea) reads the stored length without detoasting payload.
			rows, e := s.Pool.Query(ctx, `SELECT id,content FROM (
 SELECT id,content,COALESCE(sum(octet_length(content)::bigint) OVER
 (ORDER BY id ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING),0) AS bytes_before
 FROM (SELECT id,content FROM gomate_packets WHERE stream_id=$1 AND id>$2 ORDER BY id LIMIT $3) p
 ) bounded WHERE bytes_before<$4 ORDER BY id`, id, after, 513-packets, (8<<20)-bytes)
			if e != nil {
				return out, e
			}
			for rows.Next() {
				var packetID int64
				var content []byte
				if e = rows.Scan(&packetID, &content); e != nil {
					rows.Close()
					return out, e
				}
				packets++
				bytes += len(content)
				if re.Match(content) {
					matched = true
					break
				}
				if packets >= 512 || bytes >= 8<<20 || time.Now().After(deadline) {
					before := id + 1
					out.Next = &model.SearchCursor{Before: &before, InStream: id, AfterPacket: packetID}
					rows.Close()
					return out, nil
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return out, e
			}
		}
		if matched {
			out.Streams = append(out.Streams, stream)
		}
		if len(out.Streams) >= search.Page.PageSize || packets >= 512 || bytes >= 8<<20 || time.Now().After(deadline) {
			return out, nil
		}
	}
	if len(streams) < 128 {
		out.Next = nil
	}
	return out, nil
}
