package anomaly

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"gomate/internal/analyze"
	"gomate/internal/model"
)

const flag = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01234="

func catalog(t testing.TB) *analyze.Catalog {
	c := analyze.NewCatalog()
	err := c.Replace([]model.Service{{Port: 80, HTTP: true}, {Port: 9000}}, []model.Pattern{{ID: 1, Enabled: true, Name: "flag", Value: `[A-Z0-9]{31}=`, Color: "#ff0000", SearchType: "REGEX", Direction: "BOTH", Action: "FIND"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var rng = rand.New(rand.NewSource(1))

func token(n int) string {
	const a = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = a[rng.Intn(len(a))]
	}
	return string(b)
}

func request(method, target, body string, headers ...string) model.Packet {
	h := "Host: notes:80\r\nUser-Agent: checker/1.0\r\n"
	if body != "" {
		h += "Content-Type: application/x-www-form-urlencoded\r\n"
	}
	for _, x := range headers {
		h += x + "\r\n"
	}
	return model.Packet{Kind: "http", Incoming: true, Content: []byte(method + " " + target + " HTTP/1.1\r\n" + h + "\r\n" + body)}
}

func response(status, body string, headers ...string) model.Packet {
	h := "Content-Type: text/plain\r\n"
	for _, x := range headers {
		h += x + "\r\n"
	}
	return model.Packet{Kind: "http", Content: []byte("HTTP/1.1 " + status + "\r\n" + h + "\r\n" + body)}
}

type harness struct {
	e  *Engine
	id int64
}

func (h *harness) session(port int, packets ...model.Packet) int64 {
	h.id++
	h.e.process(&model.Chunk{Stream: model.Stream{ID: h.id, Key: fmt.Sprint("s", h.id), Service: port, Protocol: "TCP"}, Packets: packets, Closed: true})
	return h.id
}

func (h *harness) result(id int64) (Result, bool) {
	r, ok := h.e.pending[id]
	return r, ok
}

func newHarness(t testing.TB) *harness {
	return &harness{e: newEngine(catalog(t), nil, time.Second)}
}

func (h *harness) login(user, pass string) int64 {
	cookie := token(24)
	return h.session(80,
		request("POST", "/api/login", "username="+user+"&password="+pass),
		response("200 OK", "ok", "Set-Cookie: session="+cookie+"; Path=/"),
		request("GET", "/api/notes/"+token(12), "", "Cookie: session="+cookie),
		response("200 OK", "note "+token(20)))
}

func train(h *harness, n int) {
	for i := 0; i < n; i++ {
		h.login(token(6+rng.Intn(5)), token(10+rng.Intn(6)))
	}
}

func hasReason(r Result, detector, field string) bool {
	for _, x := range r.Reasons {
		if x.Detector == detector && (field == "" || x.Field == field) {
			return true
		}
	}
	return false
}

func TestNormalSessionsStayQuiet(t *testing.T) {
	h := newHarness(t)
	train(h, 300)
	clear(h.e.pending)
	quiet := 0
	for i := 0; i < 100; i++ {
		if _, ok := h.result(h.login(token(8), token(12))); !ok {
			quiet++
		}
	}
	if quiet < 97 {
		t.Fatalf("only %d/100 normal sessions stayed below the report threshold: %+v", quiet, h.e.pending)
	}
}

func TestInjectionIsExplained(t *testing.T) {
	h := newHarness(t)
	train(h, 300)
	for _, tc := range []struct{ user, detector string }{
		{"admin' OR '1'='1", "chars"},
		{"{{7*7}}", "chars"},
		{"x;cat /flag", "chars"},
	} {
		id := h.login(tc.user, token(12))
		r, ok := h.result(id)
		if !ok || r.Score < 10 || !hasReason(r, tc.detector, "form.username") {
			t.Fatalf("%q: expected %s reason on form.username, got %+v", tc.user, tc.detector, r)
		}
		t.Logf("%q score=%.1f", tc.user, r.Score)
		for _, x := range r.Reasons {
			t.Logf("  +%.1f %s %s: %s", x.Bits, x.Detector, x.Field, x.Text)
		}
		if r.Summary != "POST /api/login" {
			t.Fatalf("summary %q", r.Summary)
		}
	}
}

func TestRepeatedAttackStaysVisible(t *testing.T) {
	h := newHarness(t)
	train(h, 300)
	var last Result
	for i := 0; i < 5; i++ {
		last, _ = h.result(h.login("admin' OR '1'='1", token(12)))
	}
	if last.Score < 10 || !hasReason(last, "chars", "form.username") {
		t.Fatalf("fifth identical injection no longer reported: %+v", last)
	}
}

func TestVariablePathAndNewEndpoint(t *testing.T) {
	h := newHarness(t)
	train(h, 300)
	if x := h.e.services[80].templates["GET /api/notes/{*}"]; x == nil || x.n < 280 {
		t.Fatalf("path not generalised: %+v", x)
	}
	id := h.session(80, request("GET", "/api/admin/debug", ""), response("200 OK", "debug"))
	if r, ok := h.result(id); !ok || !hasReason(r, "template", "") {
		t.Fatalf("new endpoint not reported: %+v", r)
	}
	id = h.session(80, request("GET", "/api/notes/..%2f..%2fetc%2fpasswd", ""), response("200 OK", "root:x:0:0"))
	if r, ok := h.result(id); !ok || !hasReason(r, "chars", "path[2]") {
		t.Fatalf("traversal in variable path segment not reported: %+v", r)
	}
}

func TestMassAssignmentAndEnum(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 200; i++ {
		h.session(80, request("POST", "/api/register", "username="+token(8)+"&password="+token(12)+"&role=user"), response("200 OK", "ok"))
	}
	id := h.session(80, request("POST", "/api/register", "username="+token(8)+"&password="+token(12)+"&role=admin&is_admin=1"), response("200 OK", "ok"))
	r, ok := h.result(id)
	if !ok || !hasReason(r, "enum", "form.role") || !hasReason(r, "field", "form.is_admin") {
		t.Fatalf("expected enum and new field reasons: %+v", r)
	}
}

func TestFlagProvenance(t *testing.T) {
	h := newHarness(t)
	train(h, 150)
	user, pass := token(8), token(12)
	put := h.session(80, request("POST", "/api/login", "username="+user+"&password="+pass), response("200 OK", "ok", "Set-Cookie: session="+token(24)),
		request("POST", "/api/notes", "text="+flag), response("200 OK", "saved"))
	if h.e.services[80].stats.FlagsPut != 1 {
		t.Fatalf("flag placement not seen")
	}
	get := h.session(80, request("POST", "/api/login", "username="+user+"&password="+pass), response("200 OK", "ok", "Set-Cookie: session="+token(24)),
		request("GET", "/api/notes/"+token(12), ""), response("200 OK", flag))
	if r, ok := h.result(get); ok && r.Kind == "theft" {
		t.Fatalf("checker retrieval reported as theft: %+v", r)
	}
	steal := h.session(80, request("GET", "/api/notes/"+token(12)+"?user="+user, ""), response("200 OK", flag))
	r, ok := h.result(steal)
	if !ok || r.Kind != "theft" || r.Score < theftBits || !strings.Contains(r.Reasons[0].Text, fmt.Sprintf("#%d", put)) {
		t.Fatalf("theft not reported: %+v", r)
	}
	s := h.e.services[80].stats
	if s.FlagsGot != 1 || s.FlagsStole != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestRawTCP(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 200; i++ {
		h.session(9000, model.Packet{Incoming: true, Content: []byte("PUT " + token(8) + " " + token(16) + "\n")}, model.Packet{Content: []byte("OK\n")})
	}
	id := h.session(9000, model.Packet{Incoming: true, Content: []byte("PUT " + token(8) + " $(cat /flag)\n")}, model.Packet{Content: []byte("OK\n")})
	if r, ok := h.result(id); !ok || !hasReason(r, "chars", "msg") {
		t.Fatalf("raw TCP injection not reported: %+v", r)
	}
}

func TestFeedbackLearns(t *testing.T) {
	h := newHarness(t)
	train(h, 300)
	r, _ := h.result(h.login("o'brien", token(12)))
	for _, x := range r.Reasons {
		h.e.services[80].accept(x)
	}
	if r, ok := h.result(h.login("d'artagnan", token(12))); ok && hasReason(r, "chars", "form.username") {
		t.Fatalf("accepted characters still reported: %+v", r)
	}
}

func BenchmarkLoginSession(b *testing.B) {
	h := newHarness(b)
	train(h, 300)
	users := make([]string, 1024)
	for i := range users {
		users[i] = token(8)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.login(users[i%len(users)], "Xa91kLm2pQwe")
		if len(h.e.pending) > 1000 {
			clear(h.e.pending)
		}
	}
}

func TestFlagRetrievedBeforePlacementIsProcessed(t *testing.T) {
	h := newHarness(t)
	train(h, 150)
	user, pass := token(8), token(12)
	steal := h.session(80, request("GET", "/api/notes/"+token(12), ""), response("200 OK", flag))
	get := h.session(80, request("POST", "/api/login", "username="+user+"&password="+pass), response("200 OK", "ok"),
		request("GET", "/api/notes/"+token(12), ""), response("200 OK", flag))
	h.session(80, request("POST", "/api/login", "username="+user+"&password="+pass), response("200 OK", "ok"),
		request("POST", "/api/notes", "text="+flag), response("200 OK", "saved"))
	if r, ok := h.result(steal); !ok || r.Kind != "theft" {
		t.Fatalf("late placement did not resolve theft: %+v", r)
	}
	if r, ok := h.result(get); ok && r.Kind == "theft" {
		t.Fatalf("checker retrieval reported as theft: %+v", r)
	}
	if s := h.e.services[80].stats; s.FlagsOther != 0 || s.FlagsGot != 1 || s.FlagsStole != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestDecodedRequestLineWithSpaces(t *testing.T) {
	m := parseHTTP1(model.Packet{Kind: "http", Incoming: true, Content: []byte("GET /?marker=x' OR 1=1-- HTTP/1.1\r\nHost: a\r\n\r\n")})
	if m.path != "/" || m.summary != "GET /?marker=x' OR 1=1--" || len(m.fields) == 0 || string(m.fields[0].value) != "x' OR 1=1--" {
		t.Fatalf("%+v", m)
	}
}
