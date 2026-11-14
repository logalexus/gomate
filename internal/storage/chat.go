package storage

import (
	"context"
	"time"
)

type ChatMessage struct {
	ID        int64     `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) Chat(ctx context.Context, after, before int64) ([]ChatMessage, error) {
	// Return the last page initially, oldest-to-newest for incremental polling.
	order := "DESC"
	if after > 0 {
		order = "ASC"
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,author,body,created_at FROM gomate_chat WHERE created_at>now()-interval '7 days' AND ($1::bigint=0 OR id>$1) AND ($2::bigint=0 OR id<$2) ORDER BY id `+order+` LIMIT 100`, after, before)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if e = rows.Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if order == "DESC" {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, rows.Err()
}
func (s *Store) SendChat(ctx context.Context, author, key, text string) (ChatMessage, error) {
	var m ChatMessage
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return m, e
	}
	defer rollback(tx)
	// Serialize ID allocation and commit so an after-cursor cannot skip an
	// earlier, still-uncommitted message from another participant.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(83472832)`); e != nil {
		return m, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO gomate_chat(author,client_key,body) VALUES($1,$2,$3) ON CONFLICT(author,client_key) DO UPDATE SET client_key=excluded.client_key RETURNING id,author,body,created_at`, author, key, text).Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt)
	if e != nil {
		return m, e
	}
	_, e = tx.Exec(ctx, `DELETE FROM gomate_chat WHERE created_at<now()-interval '7 days' OR id<COALESCE((SELECT id FROM gomate_chat ORDER BY id DESC OFFSET 9999 LIMIT 1),0)`)
	if e != nil {
		return m, e
	}
	return m, tx.Commit(ctx)
}
