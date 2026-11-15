package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gomate/internal/storage"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type chatBucket struct {
	at     time.Time
	tokens float64
}
type chatLimiter struct {
	mu      sync.Mutex
	clients map[string]chatBucket
	global  chatBucket
}

func refill(b chatBucket, now time.Time, capacity, rate float64) chatBucket {
	if b.at.IsZero() {
		return chatBucket{now, capacity}
	}
	b.tokens = min(capacity, b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now
	return b
}
func (l *chatLimiter) allow(author string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.clients == nil {
		l.clients = map[string]chatBucket{}
	}
	if len(l.clients) >= 4096 {
		for k, v := range l.clients {
			if now.Sub(v.at) > time.Minute {
				delete(l.clients, k)
			}
		}
		if _, ok := l.clients[author]; !ok && len(l.clients) >= 4096 {
			return false
		}
	}
	b := refill(l.clients[author], now, 6, 1)
	g := refill(l.global, now, 40, 20)
	if b.tokens < 1 || g.tokens < 1 {
		return false
	}
	b.tokens--
	g.tokens--
	l.clients[author] = b
	l.global = g
	return true
}
func (s *Server) chatAuthor(w http.ResponseWriter, r *http.Request) string {
	sign := func(id string) string {
		h := hmac.New(sha256.New, []byte(s.Config.Password))
		h.Write([]byte("gomate-chat:" + id))
		return hex.EncodeToString(h.Sum(nil))
	}
	if c, e := r.Cookie("gomate_chat"); e == nil {
		parts := strings.Split(c.Value, ".")
		if len(parts) == 2 && len(parts[0]) == 32 && hmac.Equal([]byte(parts[1]), []byte(sign(parts[0]))) {
			return "Аноним-" + parts[0][:8]
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "gomate_chat", Value: id + "." + sign(id), Path: "/api/v2/chat", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 7 * 86400})
	return "Аноним-" + id[:8]
}

// Keep stored author keys stable so pre-upgrade cookies, message ownership and
// retries retain their identity. Only the display name is translated.
func chatDisplayAuthor(author string) string {
	id, legacy := strings.CutPrefix(author, "Аноним-")
	if decoded, err := hex.DecodeString(id); legacy && err == nil && len(decoded) == 4 {
		return "Anonymous-" + id
	}
	return author
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" && r.Method != "POST" {
		return &httpError{405, "method not allowed"}
	}
	author := s.chatAuthor(w, r)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		parse := func(key string) (int64, error) {
			v := r.URL.Query().Get(key)
			if v == "" {
				return 0, nil
			}
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil || n < 0 {
				return 0, bad("Invalid chat cursor")
			}
			return n, nil
		}
		after, e := parse("after")
		if e != nil {
			return e
		}
		before, e := parse("before")
		if e != nil {
			return e
		}
		if after > 0 && before > 0 {
			return bad("Specify only one cursor")
		}
		revision, e := parse("revision")
		if e != nil {
			return e
		}
		changes := []storage.ChatMessage{}
		if r.URL.Query().Has("revision") {
			changes, e = s.Store.ChatChanges(r.Context(), revision)
			if len(changes) > 0 {
				revision = changes[len(changes)-1].Revision
			}
		} else {
			revision, e = s.Store.ChatRevision(r.Context())
		}
		if e != nil {
			return e
		}
		messages := []storage.ChatMessage{}
		if r.URL.Query().Get("changesOnly") != "1" {
			messages, e = s.Store.Chat(r.Context(), after, before)
		}
		if e != nil {
			return e
		}
		for i := range messages {
			messages[i].Author = chatDisplayAuthor(messages[i].Author)
		}
		for i := range changes {
			changes[i].Author = chatDisplayAuthor(changes[i].Author)
		}
		respond(w, 200, map[string]any{"messages": messages, "author": chatDisplayAuthor(author), "hasMore": len(messages) == 100, "changes": changes, "revision": revision, "hasChangesMore": len(changes) == 100})
		return nil
	}
	var body struct {
		Text string `json:"text"`
		Key  string `json:"key"`
	}
	if e := decode(w, r, &body); e != nil {
		return e
	}
	body.Text = strings.TrimSpace(body.Text)
	if !validChatText(body.Text) {
		return bad("Message must contain 1 to 2000 characters")
	}
	if len(body.Key) < 16 || len(body.Key) > 64 {
		return bad("Invalid message key")
	}
	if !s.chatLimit.allow(author) {
		w.Header().Set("Retry-After", "1")
		return &httpError{429, "Too many messages; wait a second"}
	}
	m, e := s.Store.SendChat(r.Context(), author, body.Key, body.Text)
	if e != nil {
		return e
	}
	s.Hub.Notify()
	m.Author = chatDisplayAuthor(m.Author)
	respond(w, 201, m)
	return nil
}

func validChatText(text string) bool {
	return utf8.ValidString(text) && len(text) <= 8000 && utf8.RuneCountInString(text) <= 2000 && text != "" && !strings.ContainsRune(text, 0)
}

func (s *Server) chatMessage(w http.ResponseWriter, r *http.Request, rawID string) error {
	if r.Method != "PATCH" && r.Method != "DELETE" {
		return &httpError{405, "method not allowed"}
	}
	id, e := strconv.ParseInt(rawID, 10, 64)
	if e != nil || id <= 0 {
		return bad("Invalid message ID")
	}
	author := s.chatAuthor(w, r)
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		Text     string `json:"text"`
		Revision *int64 `json:"revision"`
	}
	if r.Method == "PATCH" {
		if e = decode(w, r, &body); e != nil {
			return e
		}
		body.Text = strings.TrimSpace(body.Text)
		if !validChatText(body.Text) {
			return bad("Message must contain 1 to 2000 characters")
		}
		if body.Revision == nil || *body.Revision < 0 {
			return bad("Message revision is required")
		}
	}
	if !s.chatLimit.allow(author) {
		w.Header().Set("Retry-After", "1")
		return &httpError{429, "Too many changes; wait a second"}
	}
	m, e := s.Store.ChangeChat(r.Context(), id, body.Text, body.Revision, r.Method == "DELETE")
	if errors.Is(e, storage.ErrChatConflict) {
		return &httpError{409, "This message was edited by someone else. Cancel and reopen Edit to use the latest version."}
	}
	if e != nil {
		return e
	}
	s.Hub.Notify()
	m.Author = chatDisplayAuthor(m.Author)
	respond(w, 200, m)
	return nil
}
