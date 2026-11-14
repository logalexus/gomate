package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockManager(t *testing.T, h http.HandlerFunc) *Manager {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(tr.CloseIdleConnections)
	m := New("", "gomate-runner:test")
	m.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { r.URL.Host = u.Host; return tr.RoundTrip(r) })}
	m.ready = true
	return m
}
func frame(stream byte, value string) []byte {
	b := make([]byte, 8)
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:], uint32(len(value)))
	return append(b, []byte(value)...)
}
func finished(t *testing.T, m *Manager, id string) Result {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, e := m.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status != "starting" && r.Status != "running" && r.Status != "stopping" {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Result{}
}

const fakeID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestScriptPolicyOutputAndCleanup(t *testing.T) {
	var spec map[string]any
	var removed atomic.Bool
	m := mockManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/containers/create":
			if e := json.NewDecoder(r.Body).Decode(&spec); e != nil {
				t.Error(e)
			}
			io.WriteString(w, `{"Id":"`+fakeID+`"}`)
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Write(frame(1, "hello\n"))
			w.Write(frame(2, "trace\n"))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			io.WriteString(w, `{"StatusCode":7}`)
		case r.Method == "DELETE":
			if !strings.HasPrefix(r.URL.Path, "/containers/gomate-run-") || r.URL.Query().Get("force") != "1" {
				t.Error(r.URL)
			}
			removed.Store(true)
			w.WriteHeader(204)
		default:
			t.Error(r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	code := "print('hello')\n"
	started, e := m.Start(Request{Language: "python", Code: code})
	if e != nil {
		t.Fatal(e)
	}
	result := finished(t, m, started.ID)
	if result.Status != "finished" || result.ExitCode == nil || *result.ExitCode != 7 || result.Stdout != "hello\n" || result.Stderr != "trace\n" || !removed.Load() {
		t.Fatalf("%+v", result)
	}
	hc := spec["HostConfig"].(map[string]any)
	if spec["User"] != "65532:65532" || spec["Image"] != "gomate-runner:test" || hc["NetworkMode"] != "host" || hc["ReadonlyRootfs"] != true || hc["Memory"] != float64(512<<20) || hc["PidsLimit"] != float64(64) || hc["NanoCpus"] != float64(500000000) || spec["OpenStdin"] != false {
		t.Fatal(spec)
	}
	if hc["Binds"] != nil || hc["Privileged"] != nil {
		t.Fatal("host access enabled", hc)
	}
	env := spec["Env"].([]any)
	for _, v := range env {
		if strings.Contains(v.(string), "TOKEN") || strings.Contains(v.(string), "PASSWORD") {
			t.Fatal("secret environment", env)
		}
	}
	cmd := spec["Cmd"].([]any)
	decoded, e := base64.StdEncoding.DecodeString(cmd[len(cmd)-1].(string))
	if e != nil || string(decoded) != code {
		t.Fatal(cmd, e)
	}
}
func TestCancelDuringCreationDoesNotStartContainer(t *testing.T) {
	creating, release := make(chan struct{}), make(chan struct{})
	var started, removed atomic.Bool
	m := mockManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/create":
			close(creating)
			<-release
			io.WriteString(w, `{"Id":"`+fakeID+`"}`)
		case strings.HasSuffix(r.URL.Path, "/start"):
			started.Store(true)
			w.WriteHeader(204)
		case r.Method == "DELETE":
			removed.Store(true)
			w.WriteHeader(204)
		default:
			t.Error(r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	j, e := m.Start(Request{Language: "bash", Code: "sleep 10"})
	if e != nil {
		t.Fatal(e)
	}
	<-creating
	if r, e := m.Cancel(j.ID); e != nil || r.Status != "stopping" {
		t.Fatal(r, e)
	}
	close(release)
	r := finished(t, m, j.ID)
	if r.Status != "canceled" || started.Load() || !removed.Load() {
		t.Fatal(r, started.Load(), removed.Load())
	}
}
func TestTimeoutKillsAndRemovesContainer(t *testing.T) {
	killed := make(chan struct{})
	var once sync.Once
	var removed atomic.Bool
	m := mockManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/create":
			io.WriteString(w, `{"Id":"`+fakeID+`"}`)
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			select {
			case <-killed:
			case <-r.Context().Done():
			}
		case strings.HasSuffix(r.URL.Path, "/wait"):
			<-r.Context().Done()
		case strings.HasSuffix(r.URL.Path, "/kill"):
			once.Do(func() { close(killed) })
			w.WriteHeader(204)
		case r.Method == "DELETE":
			removed.Store(true)
			w.WriteHeader(204)
		default:
			t.Error(r.Method, r.URL)
		}
	})
	j, e := m.Start(Request{Language: "python", Code: "while True: pass", TimeoutSeconds: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := finished(t, m, j.ID)
	if r.Status != "timed_out" || !removed.Load() {
		t.Fatal(r)
	}
	select {
	case <-killed:
	default:
		t.Fatal("not killed")
	}
}
func TestOutputBoundAndFrameStreaming(t *testing.T) {
	m := mockManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/create":
			io.WriteString(w, `{"Id":"`+fakeID+`"}`)
		case strings.HasSuffix(r.URL.Path, "/start"), r.Method == "DELETE":
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Write(frame(1, strings.Repeat("x", MaxOutputBytes+30000)))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			io.WriteString(w, `{"StatusCode":0}`)
		default:
			t.Error(r.Method, r.URL)
		}
	})
	j, _ := m.Start(Request{Language: "python", Code: "print('large')"})
	r := finished(t, m, j.ID)
	if r.Status != "finished" || len(r.Stdout) != MaxOutputBytes || r.Stderr != "" || !r.Truncated {
		t.Fatal(r.Status, len(r.Stdout), r.Truncated)
	}
}
func TestConcurrencyValidationAndRetention(t *testing.T) {
	m := New("", "")
	m.ready = true
	m.active = 2
	if _, e := m.Start(Request{Language: "bash", Code: "true"}); e == nil || e.(*Error).Code != 429 {
		t.Fatal(e)
	}
	for _, r := range []Request{{Language: "ruby", Code: "x"}, {Language: "bash"}, {Language: "python", Code: strings.Repeat("x", MaxCodeBytes+1)}, {Language: "bash", Code: "x", TimeoutSeconds: 121}} {
		if _, e := m.Start(r); e == nil || e.(*Error).Code != 400 {
			t.Fatal(r, e)
		}
	}
	m.jobs["old"] = &job{finished: time.Now().Add(-16 * time.Minute)}
	if _, e := m.Get("old"); e == nil {
		t.Fatal("expired job remained")
	}
	long := strings.Repeat("x", MaxCodeBytes)
	cmd := m.containerSpec(Request{Language: "python", Code: long, TimeoutSeconds: 30})["Cmd"].([]string)
	var encoded string
	for _, part := range cmd[7:] {
		if len(part) > 48<<10 {
			t.Fatal("oversized argv")
		}
		encoded += part
	}
	data, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || string(data) != long {
		t.Fatal(len(data), e)
	}
}
func TestStartupRemovesOnlyOwnedContainers(t *testing.T) {
	var deleted []string
	m := mockManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if !strings.Contains(r.URL.Query().Get("filters"), owner) {
				t.Error(r.URL)
			}
			json.NewEncoder(w).Encode([]any{map[string]any{"Id": fakeID, "Labels": map[string]string{"gomate.runner.owner": owner}}, map[string]any{"Id": strings.Repeat("b", 64), "Labels": map[string]string{"gomate.internal": "true"}}})
		} else {
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(204)
		}
	})
	if e := m.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(deleted) != 1 || deleted[0] != "/containers/"+fakeID {
		t.Fatal(deleted)
	}
}
func TestHTTPRejectsUnknownDockerOptionsAndInvalidRoutes(t *testing.T) {
	m := New("", "")
	m.ready = true
	h := m.Handler()
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{{"POST", "/runner/jobs", `{"language":"bash","code":"true","image":"evil"}`, 400}, {"POST", "/runner/jobs", `{"language":"bash","code":"true"} {}`, 400}, {"POST", "/runner/jobs", `{"language":"bash","code":""}`, 400}, {"DELETE", "/runner/jobs/../../app", "", 404}, {"POST", "/runner/containers/create", "{}", 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body)))
		if w.Code != tc.code {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
}
