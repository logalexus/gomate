package storage

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type ChatMessage struct {
	ID        int64      `json:"id"`
	Author    string     `json:"author"`
	Text      string     `json:"text"`
	CreatedAt time.Time  `json:"createdAt"`
	Revision  int64      `json:"revision"`
	EditedAt  *time.Time `json:"editedAt,omitempty"`
	Deleted   bool       `json:"deleted"`
}

func (s *Store) Chat(ctx context.Context, after, before int64) ([]ChatMessage, error) {
	// Return the last page initially, oldest-to-newest for incremental polling.
	order := "DESC"
	if after > 0 {
		order = "ASC"
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,author,body,created_at,revision,edited_at,deleted FROM gomate_chat WHERE NOT deleted AND created_at>now()-interval '7 days' AND ($1::bigint=0 OR id>$1) AND ($2::bigint=0 OR id<$2) ORDER BY id `+order+` LIMIT 100`, after, before)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if e = rows.Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt, &m.Revision, &m.EditedAt, &m.Deleted); e != nil {
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
	e = tx.QueryRow(ctx, `INSERT INTO gomate_chat(author,client_key,body) VALUES($1,$2,$3) ON CONFLICT(author,client_key) DO UPDATE SET client_key=excluded.client_key RETURNING id,author,body,created_at,revision,edited_at,deleted`, author, key, text).Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt, &m.Revision, &m.EditedAt, &m.Deleted)
	if e != nil {
		return m, e
	}
	_, e = tx.Exec(ctx, `DELETE FROM gomate_chat WHERE created_at<now()-interval '7 days' OR id<COALESCE((SELECT id FROM gomate_chat ORDER BY id DESC OFFSET 9999 LIMIT 1),0)`)
	if e != nil {
		return m, e
	}
	if m.Deleted {
		m.Text = ""
	}
	return m, tx.Commit(ctx)
}

// Read the watermark before reading a history page: a concurrent mutation will
// either be in that page or have a revision greater than the returned watermark.
func (s *Store) ChatRevision(ctx context.Context) (int64, error) {
	var revision int64
	e := s.Pool.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM gomate_chat`).Scan(&revision)
	return revision, e
}

func (s *Store) ChatChanges(ctx context.Context, revision int64) ([]ChatMessage, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id,author,body,created_at,revision,edited_at,deleted FROM gomate_chat WHERE revision>$1 AND created_at>now()-interval '7 days' ORDER BY revision LIMIT 100`, revision)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if e = rows.Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt, &m.Revision, &m.EditedAt, &m.Deleted); e != nil {
			return nil, e
		}
		if m.Deleted {
			m.Text = ""
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

var ErrChatConflict = errors.New("chat message changed")

// Every participant can change any message. The shared lock orders revisions
// with commits, including SendChat, so polling never skips a pending mutation.
func (s *Store) ChangeChat(ctx context.Context, id int64, text string, expectedRevision *int64, remove bool) (ChatMessage, error) {
	var m ChatMessage
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return m, e
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(83472832)`); e != nil {
		return m, e
	}
	e = tx.QueryRow(ctx, `SELECT id,author,body,created_at,revision,edited_at,deleted FROM gomate_chat WHERE id=$1 AND created_at>now()-interval '7 days'`, id).Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt, &m.Revision, &m.EditedAt, &m.Deleted)
	if e != nil {
		return m, e
	}
	if m.Deleted {
		if !remove {
			return m, pgx.ErrNoRows
		}
		m.Text = ""
		return m, tx.Commit(ctx)
	}
	if !remove && (expectedRevision == nil || *expectedRevision != m.Revision) {
		return m, ErrChatConflict
	}
	if remove {
		text = "[deleted]"
	}
	e = tx.QueryRow(ctx, `UPDATE gomate_chat SET body=$2,deleted=$3,edited_at=CASE WHEN $3 THEN edited_at ELSE now() END,revision=nextval('gomate_chat_revision_seq') WHERE id=$1 RETURNING id,author,body,created_at,revision,edited_at,deleted`, id, text, remove).Scan(&m.ID, &m.Author, &m.Text, &m.CreatedAt, &m.Revision, &m.EditedAt, &m.Deleted)
	if e != nil {
		return m, e
	}
	if remove {
		m.Text = ""
	}
	return m, tx.Commit(ctx)
}
