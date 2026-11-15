package runner

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (m *Manager) Handler() http.Handler {
	reads := make(chan struct{}, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case reads <- struct{}{}:
			defer func() { <-reads }()
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]string{"error": "Runner busy, try again"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		path := strings.TrimSuffix(r.URL.Path, "/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		var result any
		var err error
		status := 200
		switch {
		case path == "/runner" && r.Method == "GET":
			result = map[string]any{"available": m.Available(r.Context()), "image": m.image, "maxCodeBytes": MaxCodeBytes, "maxOutputBytes": MaxOutputBytes, "maxConcurrent": MaxConcurrent, "maxTimeoutSeconds": MaxTimeoutSeconds, "defaultTimeoutSeconds": 30, "interactive": false}
		case path == "/runner/jobs" && r.Method == "POST":
			var req Request
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if e := decoder.Decode(&req); e != nil {
				err = &Error{400, "Invalid run JSON"}
				break
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = &Error{400, "Expected a single JSON object"}
				break
			}
			result, err = m.Start(req)
			status = 202
		case len(parts) == 3 && parts[0] == "runner" && parts[1] == "jobs" && validID.MatchString(parts[2]):
			switch r.Method {
			case "GET":
				result, err = m.Get(parts[2])
			case "DELETE":
				result, err = m.Cancel(parts[2])
			default:
				err = &Error{405, "method not allowed"}
			}
		default:
			err = &Error{404, "not found"}
		}
		if err != nil {
			status = 503
			message := "Could not run the script"
			var e *Error
			if errors.As(err, &e) {
				status, message = e.Code, e.Message
			}
			result = map[string]string{"error": message}
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(result)
	})
}
