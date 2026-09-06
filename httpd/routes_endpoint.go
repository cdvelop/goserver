package httpd

import (
	"webtyp.com/router"
)

// RoutesPath is where this server exposes its route table. It is
// router.IntrospectionPath: the path is the ecosystem's, not this
// implementation's, so an operator asks the same question of a dev server and
// of a deployed Worker.
const RoutesPath = router.IntrospectionPath

func (s *Server) registerRoutesEndpoint() {
	if !s.config.RoutesEndpoint {
		return
	}
	if s.routesEndpointMounted {
		return
	}
	s.routesEndpointMounted = true
	router.MountIntrospection(s.router, RoutesPath, s.config.Policy).Public()
}
