package httpd

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerRoutesEndpoint() {
	if !s.config.RoutesEndpoint {
		return
	}

	s.mux.HandleFunc(RoutesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		routes := s.router.Routes()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(routes)
	})
}
