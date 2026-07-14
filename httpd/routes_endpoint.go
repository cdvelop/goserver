package httpd

import (
	"net/http"

	"github.com/tinywasm/json"
	"github.com/tinywasm/model"
	"github.com/tinywasm/router"
)

// routesResponse is the encodable envelope of the introspection endpoint.
//
// It exists so the routes are serialized through the DECLARED shape of RouteInfo
// (RouteInfo.EncodeFields) instead of by reflection over its Go fields. Reflection got it
// wrong, and wrong in the worst possible direction: Access and Action are numeric types,
// and the ZERO value of Access is AccessGuarded — so the MOST protected route in the
// server reported itself as `"Access":0`, which any human or agent reading this endpoint
// takes for "nothing declared". An endpoint whose only job is to expose the security
// posture of a server must never invert it. Now they travel as "guarded" and "ru".
type routesResponse []router.RouteInfo

func (rs routesResponse) IsNil() bool { return rs == nil }

func (rs routesResponse) EncodeFields(w model.FieldWriter) {
	a := w.Array("routes", len(rs))
	for _, r := range rs {
		a.Object(r)
	}
	a.Close()
}

func (s *Server) registerRoutesEndpoint() {
	if !s.config.RoutesEndpoint {
		return
	}

	s.mux.HandleFunc(pattern(router.RouteInfo{Method: http.MethodGet, Path: RoutesPath}), func(w http.ResponseWriter, r *http.Request) {
		var out string
		if err := json.Encode(routesResponse(s.router.Routes()), &out); err != nil {
			// Never swallowed: this endpoint is what an operator reads to know what is
			// exposed. A silent empty body would look like "no routes".
			http.Error(w, "encoding routes failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(out))
	})
}
