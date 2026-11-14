package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Container struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
	Ports  []struct {
		IP      string `json:"IP"`
		Private int    `json:"PrivatePort"`
		Public  int    `json:"PublicPort"`
		Type    string `json:"Type"`
	} `json:"Ports"`
}
type Candidate struct {
	ID          string `json:"id"`
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	HostIP      string `json:"hostIP"`
	Port        int    `json:"port"`
	PrivatePort int    `json:"privatePort"`
	Protocol    string `json:"protocol"`
	Available   bool   `json:"available"`
	Reason      string `json:"reason,omitempty"`
}

func Candidates(cs []Container) []Candidate {
	out := []Candidate{}
	seen := map[string]bool{}
	for _, c := range cs {
		internalProject := c.Labels["com.docker.compose.project"] == "gomate" || c.Labels["com.docker.compose.project"] == "gomate-lab"
		internalService := c.Labels["com.docker.compose.service"] == "app" || c.Labels["com.docker.compose.service"] == "db" || c.Labels["com.docker.compose.service"] == "discovery"
		if c.State != "running" || c.Labels["gomate.internal"] == "true" || internalProject && internalService {
			continue
		}
		name := c.Labels["com.docker.compose.service"]
		if name == "" && len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			name = c.ID[:min(12, len(c.ID))]
		}
		if len(c.Ports) == 0 {
			out = append(out, Candidate{ID: c.ID, ContainerID: c.ID, Name: name, Image: c.Image, Reason: "Docker does not report ports for this container"})
		}
		for _, p := range c.Ports {
			key := fmt.Sprintf("%s/%s/%d/%d/%s", c.ID, p.Type, p.Public, p.Private, p.IP)
			if seen[key] {
				continue
			}
			seen[key] = true
			v := Candidate{ID: key, ContainerID: c.ID, Name: name, Image: c.Image, HostIP: p.IP, Port: p.Public, PrivatePort: p.Private, Protocol: strings.ToUpper(p.Type), Available: p.Public > 0 && (p.Type == "tcp" || p.Type == "udp")}
			if !v.Available {
				v.Reason = "Port is not published on the host; use manual configuration for bridge capture"
			}
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Port < out[j].Port
	})
	return out
}
func Docker(ctx context.Context, socket string) ([]Candidate, error) {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	r, e := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/json", nil)
	if e != nil {
		return nil, e
	}
	resp, e := client.Do(r)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker returned HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 4<<20 {
		return nil, fmt.Errorf("container metadata exceeds 4 MiB")
	}
	var cs []Container
	if e = json.Unmarshal(b, &cs); e != nil {
		return nil, e
	}
	return Candidates(cs), nil
}
func Fetch(ctx context.Context, endpoint, token string) ([]Candidate, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("Docker discovery is not configured")
	}
	r, e := http.NewRequestWithContext(ctx, "GET", endpoint+"/services", nil)
	if e != nil {
		return nil, e
	}
	r.Header.Set("Authorization", "Bearer "+token)
	resp, e := (&http.Client{Timeout: 3 * time.Second}).Do(r)
	if e != nil {
		return nil, fmt.Errorf("Docker discovery is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker discovery returned HTTP %d", resp.StatusCode)
	}
	var out []Candidate
	e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out)
	return out, e
}
