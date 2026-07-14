package httpd

import (
	"encoding/json"
	"net/http"

	"github.com/tinywasm/router"
)

func (s *Server) registerRoutesEndpoint() {
	if !s.config.RoutesEndpoint {
		return
	}

	s.mux.HandleFunc(pattern(router.RouteInfo{Method: http.MethodGet, Path: RoutesPath}), func(w http.ResponseWriter, r *http.Request) {
		routes := s.router.Routes()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(routes)
	})
}
