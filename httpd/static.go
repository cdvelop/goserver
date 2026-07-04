package httpd

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/tinywasm/router"
)

func (s *Server) wrapWithBatteries(handler http.Handler) http.Handler {
	publicDir := s.config.PublicDir
	var fs http.Handler
	if publicDir != "" {
		fs = http.FileServer(http.Dir(publicDir))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a proxy ResponseWriter to catch 404
		rec := &statusRecorder{ResponseWriter: w}
		handler.ServeHTTP(rec, r)

		if rec.notFound && fs != nil {
			// Try static files
			path := filepath.Join(publicDir, filepath.Clean(r.URL.Path))
			shouldServe := false
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				shouldServe = true
			} else if err == nil && info.IsDir() {
				indexPath := filepath.Join(path, "index.html")
				if _, err := os.Stat(indexPath); err == nil {
					shouldServe = true
				}
			}

			if shouldServe {
				s.serveWithGlobalBatteries(w, r, fs)
				return
			}

			// If still not found, send the original 404
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 page not found\n"))
		}
	})
}

func (s *Server) serveWithGlobalBatteries(w http.ResponseWriter, r *http.Request, fs http.Handler) {
	ctx := &httpContext{w: w, r: r}
	h := func(ctx router.Context) {
		fs.ServeHTTP(w, r)
	}
	if s.config.NoCache {
		h = NoCache(h)
	}
	if s.config.Gzip {
		h = Gzip(h)
	}
	h(ctx)
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
