package farm

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestConfigTargets(t *testing.T) {
	c := Default()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	if c.Protocol != "ructf_http" {
		t.Fatal(c.Protocol)
	}
	c.Pattern = "10.60.{n}.3"
	c.First = 1
	c.Last = 50
	c.Exclude = []string{"10.60.1.3"}
	c.Targets = []Target{{"Extra", "10.60.70.3"}}
	targets, e := c.Expanded()
	if e != nil || len(targets) != 50 {
		t.Fatalf("%d %v", len(targets), e)
	}
	for _, v := range targets {
		if v.Host == "10.60.1.3" {
			t.Fatal("excluded target")
		}
	}
	c.Last = 100000
	if _, e = c.Expanded(); e == nil {
		t.Fatal("unbounded targets")
	}
	c = Default()
	c.FlagFormat = "a*"
	if c.Validate() == nil {
		t.Fatal("empty matching regex")
	}
	c.FlagFormat = "("
	if c.Validate() == nil {
		t.Fatal("invalid regex")
	}
}
func TestHTTPProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/volga/submit" {
			w.Write([]byte("ERROR_RATELIMIT"))
			return
		}
		if r.Method != "PUT" || r.Header.Get("X-Team-Token") != "test-token" {
			t.Error("protocol or token")
		}
		var flags []string
		json.NewDecoder(r.Body).Decode(&flags)
		if len(flags) != 3 {
			t.Error(flags)
		}
		json.NewEncoder(w).Encode([]map[string]string{{"flag": "c", "msg": "[c] no such flag"}, {"flag": "a", "msg": "[a] accepted"}, {"flag": "b", "msg": "[b] invalid flag"}})
	}))
	defer server.Close()
	c := Default()
	c.URL = server.URL
	c.Token = "test-token"
	got := submit(context.Background(), server.Client(), c, []Flag{{Flag: "a"}, {Flag: "b"}, {Flag: "c"}})
	for i, s := range []string{"ACCEPTED", "REJECTED", "QUEUED"} {
		if got[i].Status != s {
			t.Fatalf("%+v", got)
		}
	}
	c.Protocol = "volga"
	c.URL = server.URL + "/volga"
	if submit(context.Background(), server.Client(), c, []Flag{{Flag: "a"}})[0].Status != "QUEUED" {
		t.Fatal("ratelimit")
	}
}
func TestPersistentQueue(t *testing.T) {
	dsn := os.Getenv("FARM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FARM_TEST_DATABASE_URL to isolated gomate_farm_test")
	}
	if !strings.HasSuffix(dsn, "/gomate_farm_test") {
		t.Fatal("requires isolated gomate_farm_test database")
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	if _, e = p.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	p.Exec(ctx, `TRUNCATE gomate_farm_flags,gomate_farm_config RESTART IDENTITY`)
	b, _ := json.Marshal(Default())
	if _, e = p.Exec(ctx, `INSERT INTO gomate_farm_config(data) VALUES($1)`, b); e != nil {
		t.Fatal(e)
	}
	s := &Service{pool: p}
	c, e := s.Config(ctx)
	if e != nil {
		t.Fatal(e)
	}
	c.Token = "secret"
	c, e = s.Save(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	public := c.Public()
	if public.Token != "" || !public.TokenSet {
		t.Fatal("secret exposed")
	}
	saved, e := s.Save(ctx, public)
	if e != nil || saved.Token != "secret" {
		t.Fatal("token not preserved", e)
	}
	if _, e = s.Save(ctx, public); e != ErrConflict {
		t.Fatal("revision conflict missing", e)
	}
	flag := strings.Repeat("A", 31) + "="
	body := `{"text":"` + flag + ` ` + flag + `"}`
	r := httptest.NewRequest("POST", "/api/v2/farm/flags", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"inserted":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if e = s.step(ctx, saved); e != nil {
		t.Fatal(e)
	}
	var status string
	p.QueryRow(ctx, `SELECT status FROM gomate_farm_flags`).Scan(&status)
	if status != "QUEUED" {
		t.Fatal("paused farm submitted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Team-Token") != "secret" {
			t.Error("missing token")
		}
		json.NewEncoder(w).Encode([]map[string]string{{"flag": flag, "msg": "accepted"}})
	}))
	defer server.Close()
	saved.Enabled = true
	saved.URL = server.URL
	if e = s.step(ctx, saved); e != nil {
		t.Fatal(e)
	}
	p.QueryRow(ctx, `SELECT status FROM gomate_farm_flags`).Scan(&status)
	if status != "ACCEPTED" {
		t.Fatal(status)
	}
	p.Exec(ctx, `UPDATE gomate_farm_flags SET status='SUBMITTING',next_at=now()-interval '1 second'`)
	saved.Enabled = false
	if e = s.step(ctx, saved); e != nil {
		t.Fatal(e)
	}
	p.QueryRow(ctx, `SELECT status FROM gomate_farm_flags`).Scan(&status)
	if status != "QUEUED" {
		t.Fatal("lease recovery", status)
	}
	p.Exec(ctx, `UPDATE gomate_farm_flags SET created_at=now()-interval '10 minutes'`)
	if e = s.step(ctx, saved); e != nil {
		t.Fatal(e)
	}
	p.QueryRow(ctx, `SELECT status FROM gomate_farm_flags`).Scan(&status)
	if status != "SKIPPED" {
		t.Fatal("expiry", status)
	}
}
