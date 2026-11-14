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
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, e := engine.List(ctx)
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
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		e := engine.Action(ctx, r.PathValue("id"), r.PathValue("action"))
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
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, err := engine.Logs(ctx, r.PathValue("id"), tail, since)
		write(w, v, err)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			write(w, nil, &Error{401, "unauthorized"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
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
