package storage

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"gomate/internal/model"
)

// Semantic edits create a new pattern version. Historical matches retain their
// original meaning; deleted old versions are hidden by the updated client.
func (s *Store) ReplacePattern(ctx context.Context, old model.Pattern, v *model.Pattern) error {
	sameService := (old.ServiceID == nil && v.ServiceID == nil) || (old.ServiceID != nil && v.ServiceID != nil && *old.ServiceID == *v.ServiceID)
	if sameService && old.Value == v.Value && old.SearchType == v.SearchType && old.Action == v.Action && old.Direction == v.Direction {
		return s.PutPattern(ctx, v)
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(tx)
	old.Deleted = true
	b, _ := json.Marshal(old)
	if _, e = tx.Exec(ctx, `UPDATE gomate_patterns SET data=$2 WHERE id=$1`, old.ID, b); e != nil {
		return e
	}
	v.ID = 0
	b, e = json.Marshal(v)
	if e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `INSERT INTO gomate_patterns(data) VALUES($1) RETURNING id`, b).Scan(&v.ID); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET status='cancelled',updated_at=now() WHERE (pattern->>'id')::integer=$1 AND status IN ('queued','running')`, old.ID)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) PatternsBulk(ctx context.Context, ids []int, action string) error {
	key := "enabled"
	value := action == "enable"
	if action == "delete" {
		key = "deleted"
		value = true
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(tx)
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM gomate_patterns WHERE id=ANY($1::bigint[]) AND NOT COALESCE((data->>'deleted')::boolean,false)`, ids).Scan(&n); e != nil {
		return e
	}
	unique := map[int]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	if n != len(unique) {
		return pgx.ErrNoRows
	}
	if _, e = tx.Exec(ctx, `UPDATE gomate_patterns SET data=jsonb_set(data,ARRAY[$2]::text[],to_jsonb($3::boolean)) WHERE id=ANY($1::bigint[])`, ids, key, value); e != nil {
		return e
	}
	if action == "delete" {
		_, e = tx.Exec(ctx, `UPDATE gomate_jobs SET status='cancelled',updated_at=now() WHERE (pattern->>'id')::bigint=ANY($1::bigint[]) AND status IN ('queued','running')`, ids)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
