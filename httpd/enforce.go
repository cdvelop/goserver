package httpd

import (
	"fmt"
)

func (s *Server) validateRBAC() error {
	for _, r := range s.router.routes {
		if r.info.Resource != "" && s.config.Authorize == nil {
			return fmt.Errorf("route %s %s requires resource %q but no Authorize is configured", r.info.Method, r.info.Path, r.info.Resource)
		}
	}
	return nil
}
