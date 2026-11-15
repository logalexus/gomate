package farm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Result struct {
	Status   string
	Response string
}

func classify(s string) Result {
	if len(s) > 2048 {
		s = s[:2048]
	}
	l := strings.ToLower(s)
	for _, w := range []string{"timeout", "game not started", "try again later", "game over", "is not up", "no such flag", "ratelimit", "rate limit"} {
		if strings.Contains(l, w) {
			return Result{"QUEUED", s}
		}
	}
	for _, w := range []string{"accepted", "congrat"} {
		if strings.Contains(l, w) {
			return Result{"ACCEPTED", s}
		}
	}
	for _, w := range []string{"bad", "wrong", "expired", "unknown", "your own", "too old", "not in database", "already submitted", "invalid flag"} {
		if strings.Contains(l, w) {
			return Result{"REJECTED", s}
		}
	}
	return Result{"QUEUED", s}
}
func request(ctx context.Context, cl *http.Client, method, url, token string, b []byte) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	if method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Team-Token", token)
	} else {
		req.Header.Set("Content-Type", "text/plain")
	}
	r, e := cl.Do(req)
	if e != nil {
		return nil, fmt.Errorf("checksystem connection failed")
	}
	defer r.Body.Close()
	data, e := io.ReadAll(io.LimitReader(r.Body, 1048577))
	if e != nil || len(data) > 1048576 {
		return nil, fmt.Errorf("invalid checksystem response")
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, fmt.Errorf("checksystem HTTP %d", r.StatusCode)
	}
	return data, nil
}
func submit(ctx context.Context, cl *http.Client, c Config, flags []Flag) []Result {
	out := make([]Result, len(flags))
	for i := range out {
		out[i] = Result{"QUEUED", "Missing checksystem verdict"}
	}
	if c.Protocol != "volga" {
		values := []string{}
		for _, f := range flags {
			values = append(values, f.Flag)
		}
		b, _ := json.Marshal(values)
		body, e := request(ctx, cl, "PUT", c.URL, c.Token, b)
		if e != nil {
			for i := range out {
				out[i].Response = e.Error()
			}
			return out
		}
		var rows []struct {
			Flag    string `json:"flag"`
			Message string `json:"msg"`
		}
		if json.Unmarshal(body, &rows) != nil {
			return out
		}
		byFlag := map[string]string{}
		for _, r := range rows {
			byFlag[r.Flag] = r.Message
		}
		for i, f := range flags {
			if msg, ok := byFlag[f.Flag]; ok {
				out[i] = classify(strings.TrimPrefix(msg, "["+f.Flag+"] "))
			}
		}
		return out
	}
	for i, f := range flags {
		b, e := request(ctx, cl, "POST", strings.TrimRight(c.URL, "/")+"/submit", "", []byte(f.Flag))
		if e != nil {
			out[i].Response = e.Error()
			continue
		}
		r := strings.TrimSpace(string(b))
		if len(r) > 2048 {
			r = r[:2048]
		}
		status := "QUEUED"
		if r == "SUCCESS" {
			status = "ACCEPTED"
		}
		switch r {
		case "ERROR_FLAG_INVALID", "ERROR_FLAG_EXPIRED", "ERROR_FLAG_YOUR_OWN", "ERROR_FLAG_SUBMITTED", "ERROR_FLAG_NOT_FOUND":
			status = "REJECTED"
		}
		out[i] = Result{status, r}
	}
	return out
}
