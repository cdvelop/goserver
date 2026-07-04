package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/tinywasm/gobuild"
	"github.com/tinywasm/gorun"
	"github.com/tinywasm/router"
	"github.com/tinywasm/server/httpd"
)

var ErrUnsupportedEvent = errors.New("server: unsupported file event, no rebuild triggered")

var contentChangeEvents = []string{"write", "create", "rename"}

type ServerStrategy interface {
	Start(wg *sync.WaitGroup) error
	Stop() error
	Restart() error
	HandleFileEvent(fileName, extension, filePath, event string) error
	Name() string
}

// --- Internal Strategy ---

type internalStrategy struct {
	handler *ServerHandler
	server  *http.Server
	mu      sync.Mutex
	running bool
}

func newInternalStrategy(h *ServerHandler) *internalStrategy {
	return &internalStrategy{
		handler: h,
	}
}

func (s *internalStrategy) Name() string {
	return "Internal"
}

func (s *internalStrategy) Start(wg *sync.WaitGroup) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		if wg != nil {
			wg.Done()
		}
		return nil
	}
	s.running = true

	// WaitGroup Done is handled at the end of this function (blocking until exit)

	mux := http.NewServeMux()
	r := httpd.NewRouter(mux)

	if len(s.handler.routes) > 0 {
		for _, registerConfig := range s.handler.routes {
			registerConfig(r)
		}
	} else {
		// Default handler if no routes provided
		r.Get("/", func(ctx router.Context) {
			ctx.SetHeader("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(ctx, "<h3>No routes registered in In-Memory Server</h3>")
		})
	}

	s.server = &http.Server{
		Addr:    ":" + s.handler.Port(),
		Handler: mux,
	}

	if s.handler.Https {
		// If HTTPS is enabled in internal mode, we use httpd's DevTLS
		// but since we already have a mux and http.Server here, we just need the certs
		// Strategies.go is in package server, so we can't easily call httpd.getOrCreateDevCert
		// unless we export it or move the logic.
		// For now, let's keep internalStrategy as is, but WaitForPortListening will use https.
	}
	s.mu.Unlock()

	s.handler.log("Starting Internal Server on port:", s.handler.Port())

	// Create listener first to know exactly when we're ready
	ln, err := net.Listen("tcp", ":"+s.handler.Port())
	if err != nil {
		s.handler.log("Internal Server listen error:", err)
		s.mu.Lock()
		s.running = false
		s.server = nil
		s.mu.Unlock()
		if wg != nil {
			wg.Done()
		}
		return err
	}

	// Signal that server is ready to accept connections and trigger browser open
	go func() {
		// Wait max 5 seconds for the internal server to actually respond
		if WaitForPortListening(s.handler.Port(), 5*time.Second, false) {
			s.handler.openBrowserOnce.Do(func() {
				if s.handler.OpenBrowser != nil {
					s.handler.OpenBrowser(s.handler.Port(), false) // Internal server is always http
				}
			})
		} else {
			s.handler.log("Warning: Internal Server port not responding, trying to open browser anyway...")
			s.handler.openBrowserOnce.Do(func() {
				if s.handler.OpenBrowser != nil {
					s.handler.OpenBrowser(s.handler.Port(), false)
				}
			})
		}
	}()

	// Capture server instance to avoid race condition with Stop() setting s.server = nil
	srv := s.server

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.handler.log("In-Memory Server error:", err)
		}
	}()

	// Block until exit signal received
	if s.handler.ExitChan != nil {
		<-s.handler.ExitChan
	}

	// Stop the server
	s.Stop()

	if wg != nil {
		wg.Done()
	}

	return nil
}

func (s *internalStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	timeout := 1 * time.Second
	if TestMode {
		timeout = 100 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		s.handler.log("Internal Server shutdown error, forcing close:", err)
		s.server.Close()
	}
	s.running = false
	s.server = nil
	s.handler.log("Internal Server stopped")
	return err
}

func (s *internalStrategy) Restart() error {
	s.handler.log("Restarting Internal Server...")
	err := s.Stop()
	if err != nil {
		return err
	}

	// Wait for port to be released (up to 2 seconds)
	waitForPortFree(s.handler.Port())

	// Note: We run Start in a goroutine because it blocks on ExitChan
	go s.Start(nil)

	// Wait for server to be ready before returning
	// This prevents race conditions where browser reload is triggered before server is up
	if !WaitForPortListening(s.handler.Port(), 5*time.Second, false) {
		return errors.New("timeout waiting for internal server restart")
	}
	return nil
}

func waitForPortFree(port string) {
	addr := ":" + port
	limit := 1 * time.Second
	if TestMode {
		limit = 100 * time.Millisecond
	}
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitForPortListening waits until the port is accepting HTTP or HTTPS connections (server is ready)
func WaitForPortListening(port string, timeout time.Duration, https bool) bool {
	// Use 127.0.0.1 to force IPv4, avoiding issues where localhost resolves to [::1] (IPv6)
	// but the server is listening on IPv4 only.
	scheme := "http"
	if https {
		scheme = "https"
	}
	url := scheme + "://127.0.0.1:" + port + "/"

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	timeoutCtx := 500 * time.Millisecond
	if TestMode {
		timeoutCtx = 50 * time.Millisecond
	}
	client := &http.Client{
		Timeout:   timeoutCtx,
		Transport: tr,
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (s *internalStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
	// In-memory server typically doesn't react to file events unless we want to hot-reload assets.
	// For now, no-op or specific logic if requested.
	return nil
}

// --- External Strategy ---

type externalStrategy struct {
	handler    *ServerHandler
	goCompiler gobuild.Compiler
	goRun      gorun.Runner
}

func newExternalStrategy(h *ServerHandler) *externalStrategy {
	// Initialize gobuild and gorun logic here, moved from old New()

	exe_ext := ""
	if runtime.GOOS == "windows" {
		exe_ext = ".exe"
	}

	// Extract output name from input file (e.g., "server.go" -> "server")
	outName := h.mainFileExternalServer
	if ext := filepath.Ext(outName); ext != "" {
		outName = outName[:len(outName)-len(ext)]
	}

	// Ensure the output directory exists
	if err := os.MkdirAll(filepath.Join(h.AppRootDir, h.OutputDir), 0755); err != nil {
		h.log("Error creating output directory:", err)
	}

	compiler := gobuild.New(&gobuild.Config{
		Command:                   "go",
		MainInputFileRelativePath: filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer),
		OutName:                   outName,
		Extension:                 exe_ext,
		CompilingArguments:        h.ArgumentsForCompilingServer,
		OutFolderRelativePath:     filepath.Join(h.AppRootDir, h.OutputDir),
		Logger:                    h.log,
		Timeout:                   120 * time.Second,
	})

	runner := gorun.New(&gorun.Config{
		ExecProgramPath:      compiler.FinalOutputPath(),
		RunArguments:         h.ArgumentsToRunServer,
		ExitChan:             h.ExitChan,
		Logger:               h.log,
		KillAllOnStop:        true,
		DisableGlobalCleanup: h.Config.DisableGlobalCleanup,
		WorkingDir:           filepath.Join(h.AppRootDir, h.OutputDir),
	})

	// Add output binary to .gitignore to prevent accidental commits
	binaryPath := filepath.Join(h.OutputDir, outName+exe_ext)
	if h.GitIgnoreAdd != nil {
		_ = h.GitIgnoreAdd(binaryPath)
	}

	return &externalStrategy{
		handler:    h,
		goCompiler: compiler,
		goRun:      runner,
	}
}

func (s *externalStrategy) Name() string {
	return "External Process"
}

func (s *externalStrategy) Start(wg *sync.WaitGroup) error {
	if err := s.startServer(); err != nil {
		if wg != nil {
			wg.Done()
		}
		return err
	}

	// Block until exit signal received
	if s.handler.ExitChan != nil {
		<-s.handler.ExitChan
	}

	// Stop the server
	s.Stop()

	if wg != nil {
		wg.Done()
	}

	return nil
}

func (s *externalStrategy) startServer() error {
	e := errors.New("startServer")

	// ALWAYS COMPILE before running
	err := s.goCompiler.CompileProgram()
	if err != nil {
		// Log the compilation error so it's visible in TUI
		s.handler.log(err.Error())
		return errors.Join(e, err)
	}

	// RUN
	err = s.goRun.RunProgram()
	if err != nil {
		// Log the run error so it's visible in TUI
		s.handler.log(err.Error())
		return errors.Join(e, err)
	}

	// Signal when server is ready in a separate goroutine (non-blocking)
	// Checks every 50ms until port is listening or 30s timeout
	go func() {
		if WaitForPortListening(s.handler.Port(), 30*time.Second, s.handler.Https) {
			//s.handler.log("Server is now accepting connections on port:", s.handler.Port())
			// Trigger browser open only once
			s.handler.openBrowserOnce.Do(func() {
				if s.handler.OpenBrowser != nil {
					s.handler.OpenBrowser(s.handler.Port(), s.handler.Https)
				}
			})
		} else {
			s.handler.log("Error:", "Server port not responding after 30s")
			// Try to open anyway on first attempt
			s.handler.openBrowserOnce.Do(func() {
				if s.handler.OpenBrowser != nil {
					s.handler.OpenBrowser(s.handler.Port(), s.handler.Https)
				}
			})
		}
	}()

	//s.handler.log("Started:", path.Join(s.handler.SourceDir, s.handler.mainFileExternalServer), "Port:", s.handler.Port())
	return nil
}

func (s *externalStrategy) Stop() error {
	if s.goRun != nil {
		s.handler.log("Stopping external server...")
		return s.goRun.StopProgram()
	}
	return nil
}

func (s *externalStrategy) Restart() error {
	s.handler.log("Restarting External Server...")
	err := s.Stop()
	if err != nil {
		return err
	}
	waitForPortFree(s.handler.Port()) // Ensure port is free

	// Compile synchronously to catch compilation errors
	if err := s.goCompiler.CompileProgram(); err != nil {
		return err
	}

	// Run synchronously so callers (and tests) observe the run before Restart returns.
	if err := s.goRun.RunProgram(); err != nil {
		s.handler.log("RunProgram failed:", err)
		return err
	}
	s.handler.log("External server restarted successfully")

	// Ensure browser opens if this is the first successful start
	go func() {
		if WaitForPortListening(s.handler.Port(), 30*time.Second, s.handler.Https) {
			s.handler.openBrowserOnce.Do(func() {
				if s.handler.OpenBrowser != nil {
					s.handler.OpenBrowser(s.handler.Port(), s.handler.Https)
				}
			})
		}
	}()

	go func() {
		// Block until exit signal received
		if s.handler.ExitChan != nil {
			<-s.handler.ExitChan
		}
		s.Stop()
	}()

	// Wait for server to be ready before returning
	// This prevents race conditions where browser reload triggers before server is up
	if !WaitForPortListening(s.handler.Port(), 30*time.Second, s.handler.Https) {
		return errors.New("timeout waiting for external server restart")
	}

	return nil
}

func (s *externalStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
	isContentChange := false
	for _, e := range contentChangeEvents {
		if e == event {
			isContentChange = true
			break
		}
	}

	if isContentChange {
		s.handler.log("Go file modified, restarting external server ...")
		err := s.Restart()
		if err != nil {
			s.handler.log("RestartServer failed:", err)
		} else {
			s.handler.log("RestartServer succeeded")
		}
		return err
	}
	return ErrUnsupportedEvent
}
