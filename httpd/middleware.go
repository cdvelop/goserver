package httpd

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/tinywasm/router"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// Gzip middleware
func Gzip(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx router.Context) {
		if !strings.Contains(ctx.GetHeader("Accept-Encoding"), "gzip") {
			next(ctx)
			return
		}

		// Access underlying http.ResponseWriter if possible
		hctx, ok := ctx.(*httpContext)
		if !ok {
			next(ctx)
			return
		}

		hctx.SetHeader("Content-Encoding", "gzip")
		gz := gzip.NewWriter(hctx.w)
		defer gz.Close()

		originalW := hctx.w
		hctx.w = &gzipResponseWriter{Writer: gz, ResponseWriter: originalW}
		defer func() { hctx.w = originalW }()

		next(ctx)
	}
}

// NoCache middleware
func NoCache(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		ctx.SetHeader("Pragma", "no-cache")
		ctx.SetHeader("Expires", "0")
		next(ctx)
	}
}
