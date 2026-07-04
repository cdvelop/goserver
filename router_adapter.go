package server

import (
	"io"
	"net/http"
	"sync"

	"github.com/tinywasm/router"
)

var (
	_ router.Context  = (*httpContext)(nil)
	_ router.Streamer = (*httpStreamer)(nil)
	_ router.Router   = (*httpRouter)(nil)
)

type httpContext struct {
	w      http.ResponseWriter
	r      *http.Request
	body   []byte
	once   sync.Once
	values map[string]any
}

func (c *httpContext) Method() string {
	return c.r.Method
}

func (c *httpContext) Path() string {
	return c.r.URL.Path
}

func (c *httpContext) Body() []byte {
	c.once.Do(func() {
		if c.r.Body != nil {
			c.body, _ = io.ReadAll(c.r.Body)
		}
	})
	return c.body
}

func (c *httpContext) GetHeader(key string) string {
	return c.r.Header.Get(key)
}

func (c *httpContext) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}

func (c *httpContext) WriteStatus(code int) {
	c.w.WriteHeader(code)
}

func (c *httpContext) Write(b []byte) (int, error) {
	return c.w.Write(b)
}

func (c *httpContext) SetValue(key string, v any) {
	if c.values == nil {
		c.values = make(map[string]any)
	}
	c.values[key] = v
}

func (c *httpContext) Value(key string) any {
	if v, ok := c.values[key]; ok {
		return v
	}
	return c.r.Context().Value(key)
}

type httpStreamer struct {
	*httpContext
}

func (s *httpStreamer) Flush() {
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}

type httpRouter struct {
	mux         *http.ServeMux
	middlewares []router.Middleware
}

func newHTTPRouter(mux *http.ServeMux) *httpRouter {
	return &httpRouter{
		mux: mux,
	}
}

func (r *httpRouter) Use(m ...router.Middleware) {
	r.middlewares = append(r.middlewares, m...)
}

func (r *httpRouter) Handle(method, path string, h router.HandlerFunc) {
	wrapped := h
	// Apply middlewares in reverse order
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		wrapped = r.middlewares[i](wrapped)
	}

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		if method != "" && req.Method != method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
}

func (r *httpRouter) Get(path string, h router.HandlerFunc)     { r.Handle(http.MethodGet, path, h) }
func (r *httpRouter) Post(path string, h router.HandlerFunc)    { r.Handle(http.MethodPost, path, h) }
func (r *httpRouter) Put(path string, h router.HandlerFunc)     { r.Handle(http.MethodPut, path, h) }
func (r *httpRouter) Delete(path string, h router.HandlerFunc)  { r.Handle(http.MethodDelete, path, h) }
func (r *httpRouter) Options(path string, h router.HandlerFunc) { r.Handle(http.MethodOptions, path, h) }

func (r *httpRouter) Stream(path string, h router.StreamFunc) {
	wrapped := func(ctx router.Context) {
		if s, ok := ctx.(*httpContext); ok {
			h(&httpStreamer{httpContext: s})
		}
	}
	// Apply middlewares
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		wrapped = r.middlewares[i](wrapped)
	}

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
}

func (r *httpRouter) Socket(path string, h router.SocketFunc) {
	// WebSockets require a specific library to upgrade the connection.
	// For now, we provide a stub or use a basic hijacking if available.
	// The tinywasm/router contract doesn't specify the upgrade mechanism.
	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "WebSocket not implemented in native adapter", http.StatusNotImplemented)
	})
}
