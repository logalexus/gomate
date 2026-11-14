package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var runnerJobPath = regexp.MustCompile(`^/api/v2/runner/jobs/[a-f0-9]{32}$`)

// Runner jobs outlive HTTP requests. Only fixed runner routes are proxied;
// no Docker URL, image, mount, environment or command configuration is exposed.
func (s *Server) runner(w http.ResponseWriter, r *http.Request) error {
	path := strings.TrimSuffix(r.URL.Path, "/")
	valid := path == "/api/v2/runner" && r.Method == "GET" || path == "/api/v2/runner/jobs" && r.Method == "POST" || runnerJobPath.MatchString(path) && (r.Method == "GET" || r.Method == "DELETE")
	if !valid {
		return &httpError{404, "not found"}
	}
	if s.Config.DiscoveryURL == "" {
		if r.Method == "GET" && path == "/api/v2/runner" {
			respond(w, 200, map[string]any{"available": false, "error": "Docker-шлюз не настроен"})
			return nil
		}
		return &httpError{503, "Docker-шлюз не настроен"}
	}
	var body []byte
	var e error
	if r.Method == "POST" {
		body, e = io.ReadAll(io.LimitReader(r.Body, 2<<20+1))
		if e != nil || len(body) > 2<<20 {
			return bad("Слишком большой запрос запуска")
		}
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimRight(s.Config.DiscoveryURL, "/")+strings.TrimPrefix(path, "/api/v2"), bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("X-Gomate-Timeout-Seconds", strconv.FormatInt(s.requestTimeoutSeconds(), 10))
	req.Header.Set("Authorization", "Bearer "+s.Config.DiscoveryToken)
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return &httpError{503, "Шлюз запуска недоступен"}
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if e != nil || len(data) > 4<<20 || !json.Valid(data) {
		return &httpError{502, "Некорректный ответ шлюза запуска"}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	_, e = w.Write(data)
	return e
}
