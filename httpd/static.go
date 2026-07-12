package httpd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinywasm/router"
)

func (s *Server) wrapWithBatteries(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a proxy ResponseWriter to catch 404
		rec := &statusRecorder{ResponseWriter: w}

		handler.ServeHTTP(rec, r)

		if rec.notFound && !rec.wroteHeader {
			// Try dynamic fallbacks from PublicDir routes
			s.router.mu.RLock()
			routes := make([]*httpRoute, len(s.router.routes))
			copy(routes, s.router.routes)
			s.router.mu.RUnlock()

			for _, route := range routes {
				if route.info.Dir == "" {
					continue
				}

				prefix := route.info.Path
				if !strings.HasPrefix(r.URL.Path, prefix) {
					continue
				}

				relPath := strings.TrimPrefix(r.URL.Path, prefix)
				fullPath := filepath.Join(route.info.Dir, filepath.Clean(relPath))

				// Path traversal protection
				absDir, err := filepath.Abs(route.info.Dir)
				if err != nil {
					continue
				}
				absPath, err := filepath.Abs(fullPath)
				if err != nil {
					continue
				}
				rel, err := filepath.Rel(absDir, absPath)
				if err != nil || strings.HasPrefix(rel, "..") {
					continue
				}

				info, err := os.Stat(fullPath)
				shouldServe := false
				if err == nil && !info.IsDir() {
					shouldServe = true
				} else if err == nil && info.IsDir() {
					indexPath := filepath.Join(fullPath, "index.html")
					if _, err := os.Stat(indexPath); err == nil {
						fullPath = indexPath
						shouldServe = true
					}
				}

				if shouldServe {
					// Serve the file directly using the original response writer.
					// wrapWithGlobalBatteries (applied in Handler()) will handle compression.
					http.ServeFile(w, r, fullPath)
					return
				}
			}

			// If still not found, send the original 404
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 page not found\n"))
		}
	})
}

func (s *Server) wrapWithGlobalBatteries(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := func(ctx router.Context) {
			hctx := ctx.(*httpContext)
			handler.ServeHTTP(hctx.w, r)
		}
		if s.config.NoCache {
			h = NoCache(h)
		}
		if s.config.Gzip {
			h = Gzip(h)
		}
		h(&httpContext{w: w, r: r})
	})
}

type statusRecorder struct {
	http.ResponseWriter
	notFound    bool
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if status == http.StatusNotFound && !r.wroteHeader {
		r.notFound = true
		return
	}
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.notFound && !r.wroteHeader {
		return len(b), nil
	}
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}
