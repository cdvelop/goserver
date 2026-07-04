package httpd

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
	_ router.Route    = (*httpRoute)(nil)
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

func (c *httpContext) SetCookie(cookie router.Cookie) {
	hc := &http.Cookie{
		Name:     cookie.Name,
		Value:    cookie.Value,
		Path:     cookie.Path,
		Domain:   cookie.Domain,
		MaxAge:   cookie.MaxAge,
		Secure:   cookie.Secure,
		HttpOnly: cookie.HttpOnly,
	}
	switch cookie.SameSite {
	case router.SameSiteLax:
		hc.SameSite = http.SameSiteLaxMode
	case router.SameSiteStrict:
		hc.SameSite = http.SameSiteStrictMode
	case router.SameSiteNone:
		hc.SameSite = http.SameSiteNoneMode
	default:
		hc.SameSite = http.SameSiteDefaultMode
	}
	http.SetCookie(c.w, hc)
}

func (c *httpContext) Cookie(name string) (router.Cookie, bool) {
	hc, err := c.r.Cookie(name)
	if err != nil {
		return router.Cookie{}, false
	}
	sameSite := router.SameSiteDefault
	switch hc.SameSite {
	case http.SameSiteLaxMode:
		sameSite = router.SameSiteLax
	case http.SameSiteStrictMode:
		sameSite = router.SameSiteStrict
	case http.SameSiteNoneMode:
		sameSite = router.SameSiteNone
	}
	return router.Cookie{
		Name:     hc.Name,
		Value:    hc.Value,
		Path:     hc.Path,
		Domain:   hc.Domain,
		MaxAge:   hc.MaxAge,
		Secure:   hc.Secure,
		HttpOnly: hc.HttpOnly,
		SameSite: sameSite,
	}, true
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
	mux               *http.ServeMux
	mu                sync.RWMutex
	middlewares       []router.Middleware
	globalMiddlewares []router.Middleware
	routes            []*httpRoute

	// Authorizer info to be used by enforced RBAC
	identify   func(ctx router.Context) string
	authorizer func(userID, resource, action string) bool
}

type httpRoute struct {
	info router.RouteInfo
}

func (r *httpRoute) Requires(resource string, action string) router.Route {
	r.info.Resource = resource
	r.info.Action = action
	return r
}

func NewRouter(mux *http.ServeMux) router.Router {
	return &httpRouter{
		mux: mux,
	}
}

func (r *httpRouter) Use(m ...router.Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, m...)
}

func (r *httpRouter) useGlobal(m ...router.Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalMiddlewares = append(r.globalMiddlewares, m...)
}

func (r *httpRouter) setAuthorizer(identify func(router.Context) string, authorizer func(string, string, string) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.identify = identify
	r.authorizer = authorizer
}

func (r *httpRouter) Handle(method, path string, h router.HandlerFunc) router.Route {
	route := &httpRoute{info: router.RouteInfo{Method: method, Path: path}}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		if method != "" && req.Method != method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		r.mu.RLock()
		identify := r.identify
		authorizer := r.authorizer
		mws := make([]router.Middleware, len(r.middlewares))
		copy(mws, r.middlewares)
		gmws := make([]router.Middleware, len(r.globalMiddlewares))
		copy(gmws, r.globalMiddlewares)
		r.mu.RUnlock()

		wrapped := h

		// 1. Apply RBAC (it must be innermost so it has access to route.info)
		// We pass route.info via a special middleware-like wrapper
		if route.info.Resource != "" {
			next := wrapped
			wrapped = func(ctx router.Context) {
				if authorizer == nil {
					http.Error(w, "Authorizer not configured", http.StatusInternalServerError)
					return
				}
				userID := ""
				if identify != nil {
					userID = identify(ctx)
				}
				if !authorizer(userID, route.info.Resource, route.info.Action) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
				next(ctx)
			}
		}

		// 2. Apply local middlewares
		for i := 0; i < len(mws); i++ {
			wrapped = mws[i](wrapped)
		}
		// 3. Apply global middlewares
		for i := 0; i < len(gmws); i++ {
			wrapped = gmws[i](wrapped)
		}

		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
	return route
}

func (r *httpRouter) Get(path string, h router.HandlerFunc) router.Route {
	return r.Handle(http.MethodGet, path, h)
}

func (r *httpRouter) Post(path string, h router.HandlerFunc) router.Route {
	return r.Handle(http.MethodPost, path, h)
}

func (r *httpRouter) Put(path string, h router.HandlerFunc) router.Route {
	return r.Handle(http.MethodPut, path, h)
}

func (r *httpRouter) Delete(path string, h router.HandlerFunc) router.Route {
	return r.Handle(http.MethodDelete, path, h)
}

func (r *httpRouter) Options(path string, h router.HandlerFunc) router.Route {
	return r.Handle(http.MethodOptions, path, h)
}

func (r *httpRouter) Stream(path string, h router.StreamFunc) router.Route {
	route := &httpRoute{info: router.RouteInfo{Method: http.MethodGet, Path: path}}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		identify := r.identify
		authorizer := r.authorizer
		mws := make([]router.Middleware, len(r.middlewares))
		copy(mws, r.middlewares)
		gmws := make([]router.Middleware, len(r.globalMiddlewares))
		copy(gmws, r.globalMiddlewares)
		r.mu.RUnlock()

		wrapped := func(ctx router.Context) {
			if s, ok := ctx.(*httpContext); ok {
				h(&httpStreamer{httpContext: s})
			}
		}

		// 1. Apply RBAC
		if route.info.Resource != "" {
			next := wrapped
			wrapped = func(ctx router.Context) {
				if authorizer == nil {
					http.Error(w, "Authorizer not configured", http.StatusInternalServerError)
					return
				}
				userID := ""
				if identify != nil {
					userID = identify(ctx)
				}
				if !authorizer(userID, route.info.Resource, route.info.Action) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
				next(ctx)
			}
		}

		// 2. Apply local middlewares
		for i := 0; i < len(mws); i++ {
			wrapped = mws[i](wrapped)
		}
		// 3. Apply global middlewares
		for i := 0; i < len(gmws); i++ {
			wrapped = gmws[i](wrapped)
		}

		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
	return route
}

func (r *httpRouter) Socket(path string, h router.SocketFunc) router.Route {
	route := &httpRoute{info: router.RouteInfo{Method: http.MethodGet, Path: path}}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "WebSocket not implemented in native adapter", http.StatusNotImplemented)
	})
	return route
}

func (r *httpRouter) Routes() []router.RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]router.RouteInfo, len(r.routes))
	for i, route := range r.routes {
		result[i] = route.info
	}
	return result
}
