package storage

import (
	"context"
	"encoding/json"
)

// Traffic groups describe retained sessions, independently of detector learning.
// Endpoint groups use the first request method/path without query values.
// Behavior groups additionally populate the fingerprint and descriptor fields.
type TrafficGroup struct {
	Fingerprint      string              `json:"fingerprint,omitempty"`
	Fields           []string            `json:"fields,omitempty"`
	VariableFields   []string            `json:"variableFields,omitempty"`
	VariableExamples map[string][]string `json:"variableExamples,omitempty"`
	Limited          bool                `json:"limited,omitempty"`
	Missing          bool                `json:"missing,omitempty"`
	Service          int                 `json:"service"`
	Protocol         string              `json:"protocol"`
	Method           string              `json:"method"`
	Path             string              `json:"path"`
	Count            int64               `json:"count"`
	AnomalyCount     int64               `json:"anomalyCount"`
	Patterns         []int               `json:"foundPatternsIds"`
	DismissedCount   int64               `json:"dismissedCount"`
	Score            float64             `json:"score"`
	Start            int64               `json:"startTimestamp"`
}

type TrafficGroupPage struct {
	Groups         []TrafficGroup `json:"groups"`
	TotalGroups    int64          `json:"totalGroups"`
	TotalSessions  int64          `json:"totalSessions"`
	TotalAnomalies int64          `json:"totalAnomalies"`
}

type TrafficGroupQuery struct {
	Service                int
	SinceMs                int64
	ByTime                 bool
	Limit                  int
	Offset                 int
	Protocol, Method, Path string
	Before                 int64
	AlertsOnly             bool
	Fingerprint            string
	ByCount                bool
}

const trafficGroupSource = ` FROM gomate_streams s
 LEFT JOIN gomate_anomalies a ON a.stream_id=s.id
 WHERE NOT s.ignored AND ($1=0 OR s.service=$1) AND s.start_ms >= $2 `

func (s *Store) TrafficGroups(ctx context.Context, q TrafficGroupQuery) (TrafficGroupPage, error) {
	order := `score DESC, "startTimestamp" DESC, service, protocol, method, path`
	if q.ByTime {
		order = `"startTimestamp" DESC, service, protocol, method, path`
	}
	var payload []byte
	err := s.Pool.QueryRow(ctx, `WITH source AS (
 SELECT s.service,s.protocol,COALESCE(s.first_request_method,'') AS method,
 split_part(COALESCE(s.first_request_path,''),'?',1) AS path,
 s.pattern_ids,s.start_ms,a.stream_id AS alert_id,a.dismissed,a.score`+trafficGroupSource+`
), groups AS (
 SELECT service,protocol,method,path,count(*) AS count,
 count(*) FILTER (WHERE alert_id IS NOT NULL AND NOT dismissed) AS "anomalyCount",
 count(*) FILTER (WHERE dismissed) AS "dismissedCount",
 COALESCE(max(score) FILTER (WHERE NOT dismissed),0) AS score,
 max(start_ms) AS "startTimestamp" FROM source
 GROUP BY service,protocol,method,path
), page AS (SELECT * FROM groups ORDER BY `+order+` LIMIT $3 OFFSET $4), patterns AS (
 SELECT service,protocol,method,path,array_agg(DISTINCT id ORDER BY id) AS "foundPatternsIds"
 FROM source JOIN page USING(service,protocol,method,path)
 CROSS JOIN LATERAL unnest(pattern_ids) AS id
 GROUP BY service,protocol,method,path
)
 SELECT jsonb_build_object(
 'groups', (SELECT COALESCE(jsonb_agg(p),'[]'::jsonb) FROM
 (SELECT page.*,COALESCE(patterns."foundPatternsIds",'{}'::integer[]) AS "foundPatternsIds"
 FROM page LEFT JOIN patterns USING(service,protocol,method,path) ORDER BY `+order+`) p),
 'totalGroups', (SELECT count(*) FROM groups),
 'totalSessions', (SELECT COALESCE(sum(count),0) FROM groups),
 'totalAnomalies', (SELECT COALESCE(sum("anomalyCount"),0) FROM groups))`,
		q.Service, q.SinceMs, q.Limit, q.Offset).Scan(&payload)
	var out TrafficGroupPage
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(payload, &out)
	return out, err
}

type TrafficSession struct {
	Anomaly
	Patterns      []int  `json:"foundPatternsIds"`
	HasAnomaly    bool   `json:"hasAnomaly"`
	PayloadPurged bool   `json:"payloadPurged"`
	Request       string `json:"request"`
}

type TrafficSessionPage struct {
	Items      []TrafficSession `json:"items"`
	NextBefore int64            `json:"nextBefore"`
}

func (s *Store) TrafficGroupSessions(ctx context.Context, q TrafficGroupQuery) (TrafficSessionPage, error) {
	out := TrafficSessionPage{Items: []TrafficSession{}}
	var ids []int64
	if q.Fingerprint != "" {
		groups, err := s.behaviorGroups(ctx, q)
		if err != nil {
			return out, err
		}
		for _, g := range groups {
			if g.Fingerprint == q.Fingerprint {
				ids = g.ids
				break
			}
		}
		if len(ids) == 0 {
			return out, nil
		}
	}

	rows, err := s.Pool.Query(ctx, `SELECT s.id,s.service,s.protocol,s.start_ms,s.client,
 s.payload_purged, trim(COALESCE(s.first_request_method,'') || ' ' || COALESCE(s.first_request_path,'')),
 a.stream_id IS NOT NULL,COALESCE(a.score,0),COALESCE(a.kind,''),COALESCE(a.summary,''),
 COALESCE(a.reasons,'[]'::jsonb),COALESCE(a.dismissed,false),COALESCE(a.updated_ms,0),s.pattern_ids`+trafficGroupSource+`
 AND s.protocol=$3 AND COALESCE(s.first_request_method,'')=$4
 AND split_part(COALESCE(s.first_request_path,''),'?',1)=$5
 AND ($6::bigint=0 OR s.id<$6) AND (NOT $7 OR (a.stream_id IS NOT NULL AND NOT a.dismissed))
 AND ($9::bigint[] IS NULL OR s.id=ANY($9))
 ORDER BY s.id DESC LIMIT $8`, q.Service, q.SinceMs, q.Protocol, q.Method, q.Path, q.Before, q.AlertsOnly, q.Limit+1, ids)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v TrafficSession
		var reasons []byte
		if err = rows.Scan(&v.StreamID, &v.Service, &v.Protocol, &v.Start, &v.Client, &v.PayloadPurged, &v.Request,
			&v.HasAnomaly, &v.Score, &v.Kind, &v.Summary, &reasons, &v.Dismissed, &v.UpdatedMs, &v.Patterns); err != nil {
			return out, err
		}
		if err = json.Unmarshal(reasons, &v.Reasons); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		out.NextBefore = out.Items[len(out.Items)-1].StreamID
	}
	return out, rows.Err()
}
