package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gomate/internal/anomaly"

	"github.com/jackc/pgx/v5"
)

type behaviorSession struct {
	id, start                  int64
	service                    int
	protocol, method, path     string
	fields                     []anomaly.BehaviorField
	structure                  string
	limited, missing           bool
	hasAlert, dismissed, theft bool
	score                      float64
}
type behaviorCluster struct {
	TrafficGroup
	ids []int64
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

// Field variability is estimated from retained sessions with the same structure,
// before applying the time filter. Values with disproportionate repetition stay
// literal in otherwise variable fields. No anomaly score enters this decision.
func clusterBehavior(ctx context.Context, sessions []behaviorSession, since int64) ([]behaviorCluster, error) {
	families := map[string][]int{}
	for i := range sessions {
		families[sessions[i].structure] = append(families[sessions[i].structure], i)
	}
	groups := []behaviorCluster{}
	for _, indices := range families {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		counts := map[string]map[string]int{}
		for _, i := range indices {
			for _, f := range sessions[i].fields {
				if counts[f.Key] == nil {
					counts[f.Key] = map[string]int{}
				}
				counts[f.Key][f.Value]++
			}
		}
		peaks := map[string]int{}
		for key, counts := range counts {
			for _, n := range counts {
				if n > peaks[key] {
					peaks[key] = n
				}
			}
		}
		byKey := map[string]int{}
		for _, i := range indices {
			s := sessions[i]
			if s.start < since {
				continue
			}
			values := make([]string, 0, len(s.fields))
			fields := make([]string, 0, len(s.fields))
			variable := []string{}
			for _, f := range s.fields {
				c := counts[f.Key]
				v := f.Value
				peak := peaks[f.Key]
				// Bursts reuse the same token many times. Many evenly used
				// values still indicate variability; an unusually frequent
				// value is retained as a distinct variant.
				diverse := len(c) >= 8 && (len(c)*2 >= len(indices) || (len(c) >= 32 && peak*5 < len(indices)))
				repeated := c[v] >= 3 && c[v]*len(c) >= 3*len(indices)
				// Syntax/method/path are never masked.
				literal := strings.HasSuffix(f.Key, "/method") || strings.HasSuffix(f.Key, "/path") || strings.HasSuffix(f.Key, "/separators") || strings.HasSuffix(f.Key, "/json.structure")
				if !literal && diverse && !repeated {
					v = "*"
					variable = append(variable, f.Key)
				} else {
					fields = append(fields, f.Key+" = "+f.Preview)
				}
				values = append(values, v)
			}
			key := digest([]any{s.structure, values})
			index, ok := byKey[key]
			if !ok {
				index = len(groups)
				byKey[key] = index
				groups = append(groups, behaviorCluster{TrafficGroup: TrafficGroup{Fingerprint: key, Service: s.service, Protocol: s.protocol, Method: s.method, Path: s.path, Fields: fields, VariableFields: variable, Limited: s.limited, Missing: s.missing}})
			}
			g := &groups[index]
			g.ids = append(g.ids, s.id)
			g.Count++
			if s.start > g.Start {
				g.Start = s.start
			}
			if s.hasAlert && !s.dismissed {
				g.AnomalyCount++
				if s.theft {
					g.TheftCount++
				}
				if s.score > g.Score {
					g.Score = s.score
				}
			}
			if s.dismissed {
				g.DismissedCount++
			}
		}
	}
	return groups, nil
}

func (s *Store) behaviorGroups(ctx context.Context, q TrafficGroupQuery) ([]behaviorCluster, error) {
	// The descriptor scan needs the same bounded read budget as its API route.
	// SET LOCAL prevents this override from leaking into pooled writer connections.
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '10s'"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id,s.service,s.protocol,s.start_ms,
 COALESCE(s.first_request_method,''),split_part(COALESCE(s.first_request_path,''),'?',1),
 COALESCE(s.behavior,'[]'::jsonb),a.stream_id IS NOT NULL,COALESCE(a.dismissed,false),COALESCE(a.kind='theft',false),COALESCE(a.score,0)
 FROM gomate_streams s LEFT JOIN gomate_anomalies a ON a.stream_id=s.id
 WHERE NOT s.ignored AND ($1=0 OR s.service=$1)`, q.Service)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []behaviorSession{}
	for rows.Next() {
		var v behaviorSession
		var payload []byte
		if err = rows.Scan(&v.id, &v.service, &v.protocol, &v.start, &v.method, &v.path, &payload, &v.hasAlert, &v.dismissed, &v.theft, &v.score); err != nil {
			return nil, err
		}
		var messages []anomaly.BehaviorMessage
		if err = json.Unmarshal(payload, &messages); err != nil {
			return nil, err
		}
		structure := []string{fmt.Sprint(v.service), v.protocol, v.method, v.path}
		v.missing = len(messages) == 0
		v.limited = len(messages) >= anomaly.BehaviorMessages
		for i, m := range messages {
			structure = append(structure, m.Kind)
			v.limited = v.limited || m.Limited
			for _, f := range m.Fields {
				f.Key = fmt.Sprintf("%d:%s/%s", i+1, m.Kind, f.Key)
				structure = append(structure, f.Key)
				v.fields = append(v.fields, f)
			}
		}
		structure = append(structure, fmt.Sprint(v.limited), fmt.Sprint(v.missing))
		v.structure = digest(structure)
		sessions = append(sessions, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return clusterBehavior(ctx, sessions, q.SinceMs)
}

func (s *Store) BehaviorGroups(ctx context.Context, q TrafficGroupQuery) (TrafficGroupPage, error) {
	out := TrafficGroupPage{Groups: []TrafficGroup{}}
	groups, err := s.behaviorGroups(ctx, q)
	if err != nil {
		return out, err
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if q.ByCount && a.Count != b.Count {
			return a.Count > b.Count
		}
		if !q.ByTime && !q.ByCount && a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Start != b.Start {
			return a.Start > b.Start
		}
		return a.Fingerprint < b.Fingerprint
	})
	out.TotalGroups = int64(len(groups))
	for i, g := range groups {
		out.TotalSessions += g.Count
		out.TotalAnomalies += g.AnomalyCount
		if i >= q.Offset && i < q.Offset+q.Limit {
			out.Groups = append(out.Groups, g.TrafficGroup)
		}
	}
	return out, nil
}
