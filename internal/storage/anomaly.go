package storage

import (
	"context"
	"encoding/json"

	"gomate/internal/anomaly"
)

type Anomaly struct {
	anomaly.Result
	Dismissed bool   `json:"dismissed"`
	Start     int64  `json:"startTimestamp"`
	Client    string `json:"client"`
	Protocol  string `json:"protocol"`
}

type AnomalyQuery struct {
	Service   int
	MinScore  float64
	SinceMs   int64
	Dismissed bool
	ByTime    bool
	Limit     int
}

// SaveAnomalies upserts the latest score of each stream. Rows of streams that
// retention already removed are skipped by the join.
func (s *Store) SaveAnomalies(ctx context.Context, rows []anomaly.Result) error {
	payload, e := json.Marshal(rows)
	if e != nil {
		return e
	}
	_, e = s.Pool.Exec(ctx, `INSERT INTO gomate_anomalies(stream_id,service,score,kind,summary,reasons,updated_ms)
 SELECT x."streamId",x.service,x.score,x.kind,x.summary,COALESCE(x.reasons,'[]'),x."updatedMs"
 FROM jsonb_to_recordset($1::jsonb) AS x("streamId" bigint,service integer,score real,kind text,summary text,reasons jsonb,"updatedMs" bigint)
 JOIN gomate_streams s ON s.id=x."streamId"
 ON CONFLICT(stream_id) DO UPDATE SET score=excluded.score,kind=excluded.kind,summary=excluded.summary,reasons=excluded.reasons,updated_ms=excluded.updated_ms`, payload)
	return e
}

func (s *Store) Anomalies(ctx context.Context, q AnomalyQuery) ([]Anomaly, error) {
	order := "a.score DESC, a.stream_id DESC"
	if q.ByTime {
		order = "a.stream_id DESC"
	}
	rows, e := s.Pool.Query(ctx, `SELECT a.stream_id,a.service,a.score,a.kind,a.summary,a.reasons,a.dismissed,a.updated_ms,s.start_ms,s.client,s.protocol
 FROM gomate_anomalies a JOIN gomate_streams s ON s.id=a.stream_id
 WHERE ($1=0 OR a.service=$1) AND a.score>=$2 AND s.start_ms>=$3 AND ($4 OR NOT a.dismissed) AND NOT s.ignored
 ORDER BY `+order+` LIMIT $5`, q.Service, q.MinScore, q.SinceMs, q.Dismissed, q.Limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Anomaly{}
	for rows.Next() {
		var v Anomaly
		var reasons []byte
		var score float32
		if e = rows.Scan(&v.StreamID, &v.Service, &score, &v.Kind, &v.Summary, &reasons, &v.Dismissed, &v.UpdatedMs, &v.Start, &v.Client, &v.Protocol); e != nil {
			return nil, e
		}
		v.Score = float64(score)
		if e = json.Unmarshal(reasons, &v.Reasons); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DismissAnomaly hides a row and returns it so the model can learn its reasons.
func (s *Store) DismissAnomaly(ctx context.Context, id int64, dismissed bool) (anomaly.Result, error) {
	var v anomaly.Result
	var reasons []byte
	var score float32
	e := s.Pool.QueryRow(ctx, `UPDATE gomate_anomalies SET dismissed=$2 WHERE stream_id=$1 RETURNING stream_id,service,score,kind,summary,reasons`, id, dismissed).Scan(&v.StreamID, &v.Service, &score, &v.Kind, &v.Summary, &reasons)
	if e != nil {
		return v, e
	}
	v.Score = float64(score)
	if e = json.Unmarshal(reasons, &v.Reasons); e != nil {
		return v, e
	}
	return v, nil
}

// VisibleAnomalyIDs excludes removed, ignored and manually dismissed sessions
// from pending notifications. The event feed bounds the number of IDs.
func (s *Store) VisibleAnomalyIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.stream_id FROM gomate_anomalies a
        JOIN gomate_streams s ON s.id=a.stream_id
        WHERE a.stream_id=ANY($1::bigint[]) AND NOT a.dismissed AND NOT s.ignored`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	visible := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		visible[id] = true
	}
	return visible, rows.Err()
}
