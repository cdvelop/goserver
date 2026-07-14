package httpd

import (
	"net/http"
	"time"

	"github.com/tinywasm/model"
	"github.com/tinywasm/router"
)

const (
	DefaultPort      = "8080"
	DefaultPublicDir = "public"
	HealthPath       = "/health"
	RoutesPath       = "/_routes"
)

type Config struct {
	Port      string
	PublicDir string // "" = no servir estáticos
	Gzip      bool
	NoCache   bool
	Health    bool
	Logger    func(...any)

	Authn          router.Middleware
	Authorize      model.Authorizer
	RoutesEndpoint bool

	TLS TLSConfig
}

type Server struct {
	config Config
	mux    *http.ServeMux
	router *httpRouter
}

func applyDefaults(c Config) Config {
	if c.Port == "" {
		c.Port = DefaultPort
	}
	return c
}

func New(c Config) *Server {
	c = applyDefaults(c)
	mux := http.NewServeMux()
	r := NewRouter(mux).(*httpRouter)

	s := &Server{
		config: c,
		mux:    mux,
		router: r,
	}

	return s
}

func (s *Server) Router() router.Router {
	return s.router
}

func (s *Server) Mount(m ...router.APIModule) *Server {
	for _, module := range m {
		module.MountAPI(s.router)
	}
	return s
}

func (s *Server) Handler() (http.Handler, error) {
	s.mux = http.NewServeMux()
	s.router.mux = s.mux

	if s.config.Health {
		s.mux.HandleFunc(pattern(router.RouteInfo{Method: http.MethodGet, Path: HealthPath}), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
	}

	s.registerRoutesEndpoint()

	// Re-register all existing routes in the new mux
	s.router.mu.RLock()
	routes := make([]*httpRoute, len(s.router.routes))
	copy(routes, s.router.routes)
	s.router.mu.RUnlock()

	for _, route := range routes {
		s.router.reRegister(route)
	}

	if err := s.validateRBAC(); err != nil {
		return nil, err
	}

	// Set authorizer for the router to use in closures
	s.router.setAuthorizer(s.config.Authn, s.config.Authorize)

	if err := s.validateTLS(); err != nil {
		return nil, err
	}

	var handler http.Handler = s.mux

	if s.config.PublicDir != "" {
		s.router.PublicDir("/", s.config.PublicDir)
	}

	// Apply fallback for PublicDir and global batteries
	handler = s.wrapWithBatteries(handler)
	handler = s.wrapWithGlobalBatteries(handler)

	return handler, nil
}

func (s *Server) ListenAndServe() error {
	handler, err := s.Handler()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if s.config.Logger != nil {
		s.config.Logger("Starting server on port", s.config.Port)
	}

	return s.listenAndServe(srv)
}

func (s *Server) log(args ...any) {
	if s.config.Logger != nil {
		s.config.Logger(args...)
	}
}
