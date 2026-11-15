package api

import (
	"net/http"

	"gomate/internal/model"
)

func (s *Server) trafficSearch(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	var search model.TrafficSearch
	if err := decode(w, r, &search); err != nil {
		return err
	}
	if err := validatePage(&search.Page); err != nil {
		return err
	}
	re, err := search.Compile()
	if err != nil {
		return bad(err.Error())
	}
	// Keep expensive history reads from occupying every database connection.
	running := s.trafficSearchRunning.Add(1)
	defer s.trafficSearchRunning.Add(-1)
	if running > 2 {
		w.Header().Set("Retry-After", "1")
		respond(w, http.StatusTooManyRequests, map[string]string{"error": "Search is busy; retry shortly."})
		return nil
	}
	result, err := s.Store.SearchTraffic(r.Context(), search, re)
	if err == nil {
		respond(w, http.StatusOK, result)
	}
	return err
}
