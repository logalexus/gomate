package api

import (
	"gomate/internal/dockerui"
	"net/http"
	"net/url"
)

func (s *Server) monitor(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return &httpError{405, "method not allowed"}
	}
	minutes := r.URL.Query().Get("minutes")
	if minutes == "" {
		minutes = "15"
	}
	id := r.URL.Query().Get("container")
	if (minutes != "5" && minutes != "15" && minutes != "60") || (id != "" && !dockerui.ValidID(id)) {
		return bad("Некорректный период или контейнер")
	}
	return s.dockerProxy(w, r, "/monitor?"+url.Values{"minutes": {minutes}, "container": {id}}.Encode())
}
