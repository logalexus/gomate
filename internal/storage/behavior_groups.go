package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gomate/internal/anomaly"

	"github.com/jackc/pgx/v5"
)

type behaviorSession struct {
	id, start              int64
	service                int
	protocol, method, path string
	fields                 []anomaly.BehaviorField
	structure              string
	limited, missing       bool
	hasAlert, dismissed    bool
	patterns               []int
	score                  float64
}
type behaviorCluster struct {
	TrafficGroup
	ids      []int64
	patterns map[int]bool
	examples map[string][]behaviorExample
}

type behaviorExample struct {
	value     string
	start, id int64
}

// Keep three distinct saved previews, newest first, without retaining all values.
// Previews are already bounded/redacted by the behavior descriptor builder.
func addBehaviorExample(examples []behaviorExample, sample behaviorExample) []behaviorExample {
	for i, previous := range examples {
		if previous.value == sample.value {
			if previous.start > sample.start || (previous.start == sample.start && previous.id >= sample.id) {
				return examples
			}
			examples = append(examples[:i], examples[i+1:]...)
			break
		}
	}
	examples = append(examples, sample)
	sort.Slice(examples, func(i, j int) bool {
		if examples[i].start != examples[j].start {
			return examples[i].start > examples[j].start
		}
		return examples[i].id > examples[j].id
	})
	if len(examples) > 3 {
		examples = examples[:3]
	}
	return examples
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
				groups = append(groups, behaviorCluster{patterns: map[int]bool{}, examples: map[string][]behaviorExample{}, TrafficGroup: TrafficGroup{Fingerprint: key, Service: s.service, Protocol: s.protocol, Method: s.method, Path: s.path, Fields: fields, VariableFields: variable, Limited: s.limited, Missing: s.missing}})
			}
			g := &groups[index]
			for j, f := range s.fields {
				if values[j] == "*" {
					g.examples[f.Key] = addBehaviorExample(g.examples[f.Key], behaviorExample{value: f.Preview, start: s.start, id: s.id})
				}
			}
			g.ids = append(g.ids, s.id)
			g.Count++
			for _, id := range s.patterns {
				g.patterns[id] = true
			}
			if s.start > g.Start {
				g.Start = s.start
			}
			if s.hasAlert && !s.dismissed {
				g.AnomalyCount++
				if s.score > g.Score {
					g.Score = s.score
				}
			}
			if s.dismissed {
				g.DismissedCount++
			}
		}
	}
	for i := range groups {
		g := &groups[i]
		if len(g.examples) > 0 {
			g.VariableExamples = make(map[string][]string, len(g.examples))
			for field, examples := range g.examples {
				for _, example := range examples {
					g.VariableExamples[field] = append(g.VariableExamples[field], example.value)
				}
			}
		}
		g.Patterns = make([]int, 0, len(g.patterns))
		for id := range g.patterns {
			g.Patterns = append(g.Patterns, id)
		}
		sort.Ints(g.Patterns)
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
	// PostgreSQL's limit must follow the remaining request budget, not a
	// second hard-coded ceiling. Never pass 0, which disables the DB limit.
	budget := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		budget = time.Until(deadline)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeoutMS := max(int64(1), budget.Milliseconds())
	if _, err = tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", fmt.Sprintf("%dms", timeoutMS)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id,s.service,s.protocol,s.start_ms,
 COALESCE(s.first_request_method,''),split_part(COALESCE(s.first_request_path,''),'?',1),
 COALESCE(s.behavior,'[]'::jsonb),a.stream_id IS NOT NULL,COALESCE(a.dismissed,false),s.pattern_ids,COALESCE(a.score,0)
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
		if err = rows.Scan(&v.id, &v.service, &v.protocol, &v.start, &v.method, &v.path, &payload, &v.hasAlert, &v.dismissed, &v.patterns, &v.score); err != nil {
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
