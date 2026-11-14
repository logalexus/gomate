package api

import (
	"context"
	"encoding/json"
	"fmt"
	"gomate/internal/dockerui"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) containers(w http.ResponseWriter, r *http.Request, p []string) error {
	if s.Config.DiscoveryURL == "" {
		return &httpError{503, "Docker-шлюз не настроен"}
	}
	path := "/containers"
	timeout := 4 * time.Second
	if len(p) == 3 {
		if r.Method != "GET" {
			return &httpError{405, "method not allowed"}
		}
	} else {
		if len(p) != 5 || !dockerui.ValidID(p[3]) {
			return bad("Некорректный адрес контейнера")
		}
		path += "/" + p[3] + "/" + p[4]
		if p[4] == "logs" && r.Method == "GET" {
			tail, e := strconv.Atoi(r.URL.Query().Get("tail"))
			if e != nil || tail < 1 || tail > 2000 {
				return bad("Число строк: 1–2000")
			}
			since, e := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
			if e != nil || since < 0 {
				return bad("Некорректное начало периода")
			}
			path += "?" + url.Values{"tail": {strconv.Itoa(tail)}, "since": {strconv.FormatInt(since, 10)}}.Encode()
		} else if r.Method == "POST" && dockerui.ValidAction(p[4]) {
			var body struct {
				Confirm bool `json:"confirm"`
			}
			if e := decode(w, r, &body); e != nil {
				return e
			}
			if (p[4] == "stop" || p[4] == "restart" || p[4] == "pause") && !body.Confirm {
				return bad("Подтвердите действие с контейнером")
			}
			timeout = 14 * time.Second
		} else {
			return &httpError{405, "Недопустимое действие"}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, r.Method, strings.TrimRight(s.Config.DiscoveryURL, "/")+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+s.Config.DiscoveryToken)
	resp, e := (&http.Client{Timeout: timeout}).Do(req)
	if e != nil {
		return &httpError{503, "Docker-шлюз недоступен или операция ещё не завершена; обновите статус"}
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if e != nil {
		return e
	}
	if len(data) > 4<<20 || !json.Valid(data) {
		return &httpError{502, "Некорректный ответ Docker-шлюза"}
	}
	if resp.StatusCode != 200 {
		var v struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &v)
		if v.Error == "" {
			v.Error = fmt.Sprintf("Docker HTTP %d", resp.StatusCode)
		}
		return &httpError{resp.StatusCode, v.Error}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
	return nil
}
