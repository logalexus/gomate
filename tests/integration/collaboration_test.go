package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"gomate/internal/api"
	"gomate/internal/config"
	"gomate/internal/events"
	"gomate/internal/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnonymousChatPersistenceIdempotencyPaginationAndBounds(t *testing.T) {
	s := database(t)
	app := &api.Server{Store: s, Hub: events.New(), Config: config.Config{Login: "test", Password: "test", DBTimeout: 3 * time.Second}}
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.SetBasicAuth("test", "test")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	first := call("GET", "/api/v2/chat", "", nil)
	if first.Code != 200 || len(first.Result().Cookies()) != 1 {
		t.Fatal(first.Code, first.Body.String())
	}
	cookie := first.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal(cookie)
	}
	send := func(text, key string, cookie *http.Cookie) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"text": text, "key": key})
		return call("POST", "/api/v2/chat", string(b), cookie)
	}
	w := send("hello <script> & привет", "test-message-key-1", cookie)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var m storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &m)
	w = send("hello <script> & привет", "test-message-key-1", cookie)
	var duplicate storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &duplicate)
	if duplicate.ID != m.ID {
		t.Fatal("duplicate retry", w.Body.String())
	}
	other := call("GET", "/api/v2/chat", "", nil).Result().Cookies()[0]
	w = send("second", "test-message-key-2", other)
	var second storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &second)
	if second.Author == m.Author {
		t.Fatal("anonymous identities shared across browsers")
	}
	app = &api.Server{Store: s, Hub: events.New(), Config: app.Config}
	w = call("GET", "/api/v2/chat?after="+fmt.Sprint(m.ID), "", cookie)
	var page struct {
		Author   string                `json:"author"`
		Messages []storage.ChatMessage `json:"messages"`
	}
	json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || page.Author != m.Author || len(page.Messages) != 1 || page.Messages[0].ID != second.ID {
		t.Fatal(w.Body.String())
	}
	w = send(strings.Repeat("ю", 2001), "test-message-key-3", cookie)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = send("  ", "test-message-key-4", cookie)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 6; i++ {
		w = send("bounded", fmt.Sprintf("test-message-key-%d", i+10), cookie)
		if w.Code != 201 {
			t.Fatal(i, w.Code, w.Body.String())
		}
	}
	w = send("rate", "test-message-key-last", cookie)
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.SendChat(ctx, "anonymous", fmt.Sprint(i), fmt.Sprint(i))
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e := s.Pool.Exec(ctx, `INSERT INTO gomate_chat(author,client_key,body) SELECT 'bulk',i::text,'history' FROM generate_series(1,10100) i`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SendChat(ctx, "anonymous", "prune", "last"); e != nil {
		t.Fatal(e)
	}
	var count int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM gomate_chat`).Scan(&count)
	if count != 10000 {
		t.Fatal(count)
	}
	all, e := s.Chat(ctx, 0, 0)
	if e != nil || len(all) != 100 {
		t.Fatal(len(all), e)
	}
	old, e := s.Chat(ctx, 0, all[0].ID)
	if e != nil || len(old) != 100 || old[99].ID >= all[0].ID {
		t.Fatal(old, e)
	}
	s.Pool.Exec(ctx, `UPDATE gomate_chat SET created_at=now()-interval '8 days'`)
	all, e = s.Chat(ctx, 0, 0)
	if e != nil || len(all) != 0 {
		t.Fatal(len(all), e)
	}
	s.SendChat(ctx, "anonymous", "new-day", "fresh")
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM gomate_chat`).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
}
func TestContainerAPIConfirmationOriginAndAllowlist(t *testing.T) {
	hits := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("gateway token missing")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer gateway.Close()
	app := &api.Server{Config: config.Config{Login: "test", Password: "test", DiscoveryURL: gateway.URL, DiscoveryToken: "secret"}}
	id := strings.Repeat("a", 64)
	for _, test := range []struct {
		path, body, origin string
		want               int
	}{{"stop", `{}`, "", 400}, {"exec", `{}`, "", 405}, {"restart", `{"confirm":true}`, "https://elsewhere.test", 403}, {"restart", `{"confirm":true}`, "", 200}} {
		r := httptest.NewRequest("POST", "/api/v2/containers/"+id+"/"+test.path, strings.NewReader(test.body))
		r.SetBasicAuth("test", "test")
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatal(test.path, w.Code, w.Body.String())
		}
	}
	if hits != 1 {
		t.Fatal(hits)
	}
	r := httptest.NewRequest("POST", "/api/v2/containers/"+id+"/stop", strings.NewReader(`{"confirm":true}`))
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestChatSharedEditingDeletionAndRevisionFeed(t *testing.T) {
	s := database(t)
	app := &api.Server{Store: s, Hub: events.New(), Config: config.Config{Login: "test", Password: "test", DBTimeout: 3 * time.Second}}
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.SetBasicAuth("test", "test")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	a := call("GET", "/api/v2/chat", "", nil).Result().Cookies()[0]
	b := call("GET", "/api/v2/chat", "", nil).Result().Cookies()[0]
	var original storage.ChatMessage
	w := call("POST", "/api/v2/chat", `{"text":"original **Markdown**","key":"shared-edit-message-1"}`, a)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &original)
	if original.Revision <= 0 {
		t.Fatal("missing revision")
	}
	path := "/api/v2/chat/" + fmt.Sprint(original.ID)
	// No ownership restriction: a different browser edits the original message.
	w = call("PATCH", path, fmt.Sprintf(`{"text":"edited\n\n`+"```go\\ncode\\n```"+`","revision":%d}`, original.Revision), b)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var edited storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &edited)
	if edited.Author != original.Author || edited.CreatedAt != original.CreatedAt || edited.EditedAt == nil || edited.Revision <= original.Revision {
		t.Fatal(edited)
	}
	w = call("PATCH", path, fmt.Sprintf(`{"text":"stale overwrite","revision":%d}`, original.Revision), a)
	if w.Code != 409 {
		t.Fatal("concurrent edit was not rejected", w.Code, w.Body.String())
	}
	w = call("PATCH", path, `{"text":"without revision"}`, a)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("PATCH", path, fmt.Sprintf(`{"text":"   ","revision":%d}`, edited.Revision), a)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page struct {
		Messages       []storage.ChatMessage `json:"messages"`
		Changes        []storage.ChatMessage `json:"changes"`
		Revision       int64                 `json:"revision"`
		HasChangesMore bool                  `json:"hasChangesMore"`
	}
	w = call("GET", fmt.Sprintf("/api/v2/chat?after=%d&revision=%d", original.ID, original.Revision), "", a)
	json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || len(page.Messages) != 0 || len(page.Changes) != 1 || page.Changes[0].Text != edited.Text || page.Revision != edited.Revision {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("DELETE", path, "", b)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var removed storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &removed)
	if !removed.Deleted || removed.Text != "" || removed.Revision <= edited.Revision {
		t.Fatal(removed)
	}
	w = call("DELETE", path, "", a)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var again storage.ChatMessage
	json.Unmarshal(w.Body.Bytes(), &again)
	if again.Revision != removed.Revision {
		t.Fatal("delete retry bumped revision")
	}
	w = call("GET", fmt.Sprintf("/api/v2/chat?changesOnly=1&revision=%d", edited.Revision), "", a)
	json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || len(page.Messages) != 0 || len(page.Changes) != 1 || !page.Changes[0].Deleted || page.Changes[0].Text != "" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("GET", "/api/v2/chat", "", a)
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Messages) != 0 {
		t.Fatal("deleted message in history", w.Body.String())
	}
	// Send retries cannot resurrect a deleted message or leak its original body.
	w = call("POST", "/api/v2/chat", `{"text":"original **Markdown**","key":"shared-edit-message-1"}`, a)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &again)
	if !again.Deleted || again.Text != "" || again.ID != original.ID {
		t.Fatal(again)
	}
	w = call("PATCH", path, fmt.Sprintf(`{"text":"resurrect","revision":%d}`, removed.Revision), b)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("DELETE", path, nil)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthenticated deletion", w.Code)
	}
	r = httptest.NewRequest("DELETE", path, nil)
	r.SetBasicAuth("test", "test")
	r.Header.Set("Origin", "https://other.example")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin deletion", w.Code)
	}
	var body string
	if e := s.Pool.QueryRow(context.Background(), `SELECT body FROM gomate_chat WHERE id=$1`, original.ID).Scan(&body); e != nil || body != "[deleted]" {
		t.Fatal("deleted text not cleared", body, e)
	}
}

func TestChatRevisionFeedPagesAndOlderMessages(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	first, e := s.SendChat(ctx, "a", "first", "older message")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 205; i++ {
		if _, e = s.SendChat(ctx, "bulk", fmt.Sprint(i), "newer message"); e != nil {
			t.Fatal(e)
		}
	}
	baseline, e := s.ChatRevision(ctx)
	if e != nil {
		t.Fatal(e)
	}
	edited, e := s.ChangeChat(ctx, first.ID, "updated outside latest page", &first.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	changes, e := s.ChatChanges(ctx, baseline)
	if e != nil || len(changes) != 1 || changes[0].ID != first.ID || changes[0].Revision != edited.Revision {
		t.Fatal(changes, e)
	}
	// A backlog is bounded to 100 per response and can be drained without gaps.
	cursor := int64(0)
	seen := map[int64]bool{}
	for {
		changes, e = s.ChatChanges(ctx, cursor)
		if e != nil {
			t.Fatal(e)
		}
		if len(changes) > 100 {
			t.Fatal("unbounded page")
		}
		if len(changes) == 0 {
			break
		}
		for _, m := range changes {
			if seen[m.ID] || m.Revision <= cursor {
				t.Fatal("duplicate or nonmonotonic revision", m)
			}
			seen[m.ID] = true
			cursor = m.Revision
		}
	}
	if len(seen) != 206 {
		t.Fatal("changes skipped", len(seen))
	}
	// Concurrent edits sharing a revision allow exactly one writer to commit.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.ChangeChat(ctx, edited.ID, fmt.Sprint(i), &edited.Revision, false)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if e == storage.ErrChatConflict {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
}
