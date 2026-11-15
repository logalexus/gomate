// Package dockerui exposes a narrow Docker API for container operations and logs.
package dockerui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidID(id string) bool { return validID.MatchString(id) }
func ValidAction(action string) bool {
	switch action {
	case "start", "stop", "restart", "pause", "unpause":
		return true
	}
	return false
}

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

type Container struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Ports     []string `json:"ports"`
	Protected bool     `json:"protected"`
}
type Engine struct{ client *http.Client }

func New(socket string) *Engine {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 8, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second}
	return &Engine{client: &http.Client{Transport: tr}}
}
func internal(labels map[string]string) bool {
	if labels["gomate.internal"] == "true" {
		return true
	}
	project, service := labels["com.docker.compose.project"], labels["com.docker.compose.service"]
	return (project == "gomate" || project == "gomate-lab") && (service == "app" || service == "db" || service == "discovery")
}
func (e *Engine) request(ctx context.Context, method, path string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(r)
	if err != nil {
		return nil, &Error{503, "Docker is unavailable or the operation timed out; refresh the status"}
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		code := 502
		if resp.StatusCode == 404 || resp.StatusCode == 409 {
			code = resp.StatusCode
		}
		return nil, &Error{code, fmt.Sprintf("Docker returned HTTP %d; refresh the container status", resp.StatusCode)}
	}
	return resp, nil
}
func boundedJSON(r io.Reader, v any) error {
	b, err := io.ReadAll(io.LimitReader(r, 4<<20+1))
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return &Error{502, "Docker response too large"}
	}
	return json.Unmarshal(b, v)
}
func (e *Engine) List(ctx context.Context) ([]Container, error) {
	resp, err := e.request(ctx, "GET", "/containers/json?all=1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []struct {
		ID                   string `json:"Id"`
		Names                []string
		Image, State, Status string
		Labels               map[string]string
		Ports                []struct {
			IP                      string
			PrivatePort, PublicPort int
			Type                    string
		}
	}
	if err = boundedJSON(resp.Body, &raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, c := range raw {
		name := c.ID[:min(12, len(c.ID))]
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		v := Container{ID: c.ID, Name: name, Image: c.Image, State: c.State, Status: c.Status, Protected: internal(c.Labels), Ports: []string{}}
		for _, p := range c.Ports {
			port := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
			if p.PublicPort > 0 {
				port = fmt.Sprintf("%s:%d → %s", p.IP, p.PublicPort, port)
			}
			v.Ports = append(v.Ports, port)
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type inspection struct {
	Config struct {
		Tty    bool
		Labels map[string]string
	}
	State struct{ Status string }
}

func (e *Engine) inspect(ctx context.Context, id string) (inspection, error) {
	var v inspection
	if !ValidID(id) {
		return v, &Error{400, "Invalid container ID"}
	}
	resp, err := e.request(ctx, "GET", "/containers/"+id+"/json")
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()
	err = boundedJSON(resp.Body, &v)
	return v, err
}
func (e *Engine) Action(ctx context.Context, id, action string) error {
	if !ValidAction(action) {
		return &Error{400, "Unsupported action"}
	}
	v, err := e.inspect(ctx, id)
	if err != nil {
		return err
	}
	if internal(v.Config.Labels) {
		return &Error{403, "Management of gomate containers is disabled"}
	}
	path := "/containers/" + id + "/" + action
	if action == "stop" || action == "restart" {
		path += "?t=5"
	}
	resp, err := e.request(ctx, "POST", path)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
func (e *Engine) Logs(ctx context.Context, id string, tail int, since int64) (LogSnapshot, error) {
	var out LogSnapshot
	v, err := e.inspect(ctx, id)
	if err != nil {
		return out, err
	}
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"}, "follow": {"0"}, "tail": {fmt.Sprint(tail)}}
	if since > 0 {
		q.Set("since", fmt.Sprint(since))
	}
	resp, err := e.request(ctx, "GET", "/containers/"+id+"/logs?"+q.Encode())
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	return ReadLogs(resp.Body, v.Config.Tty, tail)
}
