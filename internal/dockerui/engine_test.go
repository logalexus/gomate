package dockerui

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func frame(stream byte, text string) []byte {
	b := make([]byte, 8)
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:], uint32(len(text)))
	return append(b, []byte(text)...)
}
func TestLogFramesTTYAndBounds(t *testing.T) {
	stamp := "2026-11-14T12:00:00.123456789Z"
	b := append(frame(1, stamp+" one"), frame(2, stamp+" \x1b[31merror\x1b[0m\n")...)
	b = append(b, frame(1, " two\n")...)
	got, e := ReadLogs(bytes.NewReader(b), false, 100)
	if e != nil || len(got.Lines) != 2 || got.Lines[0].Stream != "stderr" || got.Lines[0].Text != "error" || got.Lines[1].Text != "one two" || got.Lines[1].Time != stamp {
		t.Fatal(got, e)
	}
	got, e = ReadLogs(strings.NewReader(stamp+" tty <script>\n"), true, 100)
	if e != nil || !got.TTY || got.Lines[0].Stream != "tty" || got.Lines[0].Text != "tty <script>" {
		t.Fatal(got, e)
	}
	got, e = ReadLogs(bytes.NewReader(frame(1, strings.Repeat("x", 20000)+"\nlast\n")), false, 100)
	if e != nil || !got.Truncated || len(got.Lines[0].Text) > maxLine+100 || got.Lines[1].Text != "last" {
		t.Fatal(got, e)
	}
	b = nil
	for i := 0; i < 3000; i++ {
		b = append(b, frame(1, "same\n")...)
	}
	got, e = ReadLogs(bytes.NewReader(b), false, 100)
	if e != nil || len(got.Lines) != 100 || !got.Truncated {
		t.Fatal(len(got.Lines), e)
	}
	ids := map[string]bool{}
	for _, l := range got.Lines {
		if ids[l.ID] {
			t.Fatal("duplicate virtual row key")
		}
		ids[l.ID] = true
	}
	got, e = ReadLogs(strings.NewReader(strings.Repeat("x", maxLogRead+10000)), true, 100)
	if e != nil || !got.Truncated || len(got.Lines) != 1 || len(got.Lines[0].Text) > maxLine+100 {
		t.Fatal(len(got.Lines), e)
	}
	if _, e = ReadLogs(bytes.NewReader(frame(3, "bad\n")), false, 100); e == nil {
		t.Fatal("invalid frame accepted")
	}
	got, e = ReadLogs(bytes.NewReader(frame(1, "unfinished")[:12]), false, 100)
	if e != nil || !got.Truncated {
		t.Fatal(got, e)
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGatewayAllowsOnlyExplicitActionsAndProtectsGomate(t *testing.T) {
	id := strings.Repeat("a", 64)
	protected := false
	var mutations []string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/"+id+"/json" {
			labels := map[string]string{}
			if protected {
				labels["gomate.internal"] = "true"
			}
			json.NewEncoder(w).Encode(map[string]any{"Config": map[string]any{"Labels": labels}})
			return
		}
		if r.URL.Path == "/containers/json" {
			fmt.Fprintf(w, `[{"Id":%q,"Names":["/test"],"State":"exited","Status":"Exited (0)","Labels":{},"Ports":[]}]`, id)
			return
		}
		if r.Method == "POST" {
			mutations = append(mutations, r.URL.RequestURI())
			w.WriteHeader(204)
			return
		}
		http.NotFound(w, r)
	}))
	defer daemon.Close()
	client := daemon.Client()
	base := client.Transport
	client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(daemon.URL, "http://")
		return base.RoundTrip(r)
	})
	engine := &Engine{client: client}
	token := strings.Repeat("k", 32)
	handler := Gateway(engine, token, http.NotFoundHandler())
	call := func(method, path, auth string) int {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if got := call("GET", "/containers", ""); got != 401 {
		t.Fatal(got)
	}
	if got := call("GET", "/containers", "Bearer "+token); got != 200 {
		t.Fatal(got)
	}
	for _, action := range []string{"start", "stop", "restart", "pause", "unpause"} {
		if got := call("POST", "/containers/"+id+"/"+action, "Bearer "+token); got != 200 {
			t.Fatal(action, got)
		}
	}
	if len(mutations) != 5 || mutations[1] != "/containers/"+id+"/stop?t=5" {
		t.Fatal(mutations)
	}
	for _, action := range []string{"exec", "kill", "delete", "prune"} {
		if got := call("POST", "/containers/"+id+"/"+action, "Bearer "+token); got != 400 {
			t.Fatal(action, got)
		}
	}
	protected = true
	if got := call("POST", "/containers/"+id+"/stop", "Bearer "+token); got != 403 {
		t.Fatal(got)
	}
	if len(mutations) != 5 {
		t.Fatal("protected mutation reached engine")
	}
	if e := engine.Action(context.Background(), "../../images", "start"); e == nil {
		t.Fatal("invalid ID allowed")
	}
	if got := call("GET", "/containers/"+id+"/logs?tail=99999&since=0", "Bearer "+token); got != 400 {
		t.Fatal(got)
	}
	if got := call("GET", "/containers/json", "Bearer "+token); got != 404 {
		t.Fatal(got)
	}
}
