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
	userID string
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

func (c *httpContext) SetUserID(id string) {
	c.userID = id
}

func (c *httpContext) UserID() string {
	return c.userID
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
	authn      router.Middleware
	authorizer func(userID, resource, action string) bool
}

type httpRoute struct {
	info router.RouteInfo
	h    router.HandlerFunc
	sh   router.StreamFunc
}

func (r *httpRoute) Requires(resource string, action string) router.Route {
	r.info.Resource = resource
	r.info.Action = action
	return r
}

func (r *httpRoute) Public() router.Route {
	r.info.Public = true
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

func (r *httpRouter) setAuthorizer(authn router.Middleware, authorizer func(string, string, string) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authn = authn
	r.authorizer = authorizer
}

func (r *httpRouter) Handle(method, path string, h router.HandlerFunc) router.Route {
	route := &httpRoute{info: router.RouteInfo{Method: method, Path: path}, h: h}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()

	r.register(route)
	return route
}

func (r *httpRouter) register(route *httpRoute) {
	r.mux.HandleFunc(route.info.Path, func(w http.ResponseWriter, req *http.Request) {
		if route.info.Method != "" && req.Method != route.info.Method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		wrapped := r.applySecurityAndMiddleware(route.h, route)
		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
}

func (r *httpRouter) reRegister(route *httpRoute) {
	if route.h != nil {
		r.register(route)
	} else if route.sh != nil {
		r.registerStream(route)
	}
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
	route := &httpRoute{info: router.RouteInfo{Method: http.MethodGet, Path: path}, sh: h}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()

	r.registerStream(route)
	return route
}

func (r *httpRouter) registerStream(route *httpRoute) {
	r.mux.HandleFunc(route.info.Path, func(w http.ResponseWriter, req *http.Request) {
		hfunc := func(ctx router.Context) {
			if s, ok := ctx.(*httpContext); ok {
				route.sh(&httpStreamer{httpContext: s})
			}
		}

		wrapped := r.applySecurityAndMiddleware(hfunc, route)
		ctx := &httpContext{w: w, r: req}
		wrapped(ctx)
	})
}

func (r *httpRouter) applySecurityAndMiddleware(h router.HandlerFunc, route *httpRoute) router.HandlerFunc {
	r.mu.RLock()
	authn := r.authn
	authorizer := r.authorizer
	mws := make([]router.Middleware, len(r.middlewares))
	copy(mws, r.middlewares)
	gmws := make([]router.Middleware, len(r.globalMiddlewares))
	copy(gmws, r.globalMiddlewares)
	r.mu.RUnlock()

	wrapped := h

	// 1. Apply RBAC (innermost)
	next := wrapped
	wrapped = func(ctx router.Context) {
		// Public routes: always allowed
		if route.info.Public {
			next(ctx)
			return
		}

		// Private by default: if it's not Public() and no permission, deny.
		// Requires explicitly called:
		if route.info.Resource != "" {
			if authorizer == nil {
				ctx.WriteStatus(http.StatusInternalServerError)
				ctx.Write([]byte("Authorizer not configured"))
				return
			}
			if !authorizer(ctx.UserID(), route.info.Resource, route.info.Action) {
				ctx.WriteStatus(http.StatusForbidden)
				ctx.Write([]byte("Forbidden\n"))
				return
			}
			next(ctx)
			return
		}

		// If neither Public() nor Requires() is set, it's a private route.
		// Anonymous users (empty UserID) are forbidden by default.
		if ctx.UserID() == "" {
			ctx.WriteStatus(http.StatusForbidden)
			ctx.Write([]byte("Forbidden\n"))
			return
		}
		next(ctx)
	}

	// 2. Apply local middlewares
	for i := 0; i < len(mws); i++ {
		wrapped = mws[i](wrapped)
	}
	// 3. Apply global middlewares
	for i := 0; i < len(gmws); i++ {
		wrapped = gmws[i](wrapped)
	}

	// 4. Apply Authn middleware (if any)
	if authn != nil {
		wrapped = authn(wrapped)
	}

	return wrapped
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
