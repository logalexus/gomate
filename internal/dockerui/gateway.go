package dockerui

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Gateway authenticates fixed container and script-runner routes; no generic Docker proxy is exposed.
func Gateway(engine *Engine, token string, services http.Handler) http.Handler {
	reads, actions := make(chan struct{}, 2), make(chan struct{}, 2)
	want := sha256.Sum256([]byte("Bearer " + token))
	mux := http.NewServeMux()
	mux.Handle("GET /services", services)
	mux.Handle("/runner", services)
	mux.Handle("/runner/", services)
	mux.HandleFunc("GET /containers", func(w http.ResponseWriter, r *http.Request) {
		v, e := engine.List(r.Context())
		write(w, v, e)
	})
	mux.HandleFunc("POST /containers/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		select {
		case actions <- struct{}{}:
			defer func() { <-actions }()
		default:
			write(w, nil, &Error{429, "Дождитесь завершения текущих действий"})
			return
		}
		e := engine.Action(r.Context(), r.PathValue("id"), r.PathValue("action"))
		write(w, map[string]bool{"ok": e == nil}, e)
	})
	mux.HandleFunc("GET /containers/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		tail, e := strconv.Atoi(r.URL.Query().Get("tail"))
		if e != nil || tail < 1 || tail > 2000 {
			write(w, nil, &Error{400, "tail должен быть от 1 до 2000"})
			return
		}
		since, e := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		if e != nil || since < 0 {
			write(w, nil, &Error{400, "Некорректное начало периода"})
			return
		}
		select {
		case reads <- struct{}{}:
			defer func() { <-reads }()
		default:
			write(w, nil, &Error{429, "Просмотр логов занят; повторите обновление"})
			return
		}
		v, err := engine.Logs(r.Context(), r.PathValue("id"), tail, since)
		write(w, v, err)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			write(w, nil, &Error{401, "unauthorized"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		seconds, err := strconv.ParseInt(r.Header.Get("X-Gomate-Timeout-Seconds"), 10, 64)
		if err != nil || seconds < 5 || seconds > 300 {
			seconds = 15
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(seconds)*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func write(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		code, message := 502, "Не удалось выполнить операцию Docker"
		var e *Error
		if errors.As(err, &e) {
			code, message = e.Code, e.Message
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	json.NewEncoder(w).Encode(v)
}
