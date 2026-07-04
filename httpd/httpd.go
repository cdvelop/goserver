package httpd

import (
	"net/http"
	"time"

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

	Identify       func(ctx router.Context) string
	Authorizer     func(userID, resource, action string) bool
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

func (s *Server) ListenAndServe() error {
	if s.config.Health {
		s.mux.HandleFunc(HealthPath, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
	}

	s.registerRoutesEndpoint()

	if err := s.validateRBAC(); err != nil {
		return err
	}

	// Set authorizer for the router to use in closures
	s.router.setAuthorizer(s.config.Identify, s.config.Authorizer)

	if err := s.validateTLS(); err != nil {
		return err
	}

	var handler http.Handler = s.mux

	// Apply static file serving and global batteries as a wrapper
	handler = s.wrapWithBatteries(handler)

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
