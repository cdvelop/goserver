package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/tinywasm/gobuild"
	"github.com/tinywasm/gorun"
)

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
	s.mu.Unlock()

	// WaitGroup Done is handled at the end of this function (blocking until exit)

	mux := http.NewServeMux()

	if len(s.handler.Routes) > 0 {
		for _, registerConfig := range s.handler.Routes {
			registerConfig(mux)
		}
	} else {
		// Default handler if no routes provided
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<h3>No routes registered in In-Memory Server</h3>")
		})
	}

	s.server = &http.Server{
		Addr:    ":" + s.handler.AppPort,
		Handler: mux,
	}

	s.handler.Logger("Starting Internal Server on port:", s.handler.AppPort)

	// Capture server instance to avoid race condition with Stop() setting s.server = nil
	srv := s.server

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.handler.Logger("In-Memory Server error:", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		s.handler.Logger("Internal Server shutdown error, forcing close:", err)
		s.server.Close()
	}
	s.running = false
	s.server = nil
	s.handler.Logger("Internal Server stopped")
	return err
}

func (s *internalStrategy) Restart() error {
	s.handler.Logger("Restarting Internal Server...")
	err := s.Stop()
	if err != nil {
		return err
	}

	// Wait for port to be released (up to 2 seconds)
	waitForPortFree(s.handler.AppPort)

	// Note: We run Start in a goroutine because it blocks on ExitChan
	go s.Start(nil)
	return nil
}

func waitForPortFree(port string) {
	addr := ":" + port
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *internalStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
	// In-memory server typically doesn't react to file events unless we want to hot-reload assets.
	// For now, no-op or specific logic if requested.
	return nil
}

// --- External Strategy ---

type externalStrategy struct {
	handler    *ServerHandler
	goCompiler *gobuild.GoBuild
	goRun      *gorun.GoRun
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
		h.Logger("Error creating output directory:", err)
	}

	compiler := gobuild.New(&gobuild.Config{
		Command:                   "go",
		MainInputFileRelativePath: filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer),
		OutName:                   outName,
		Extension:                 exe_ext,
		CompilingArguments:        h.ArgumentsForCompilingServer,
		OutFolderRelativePath:     filepath.Join(h.AppRootDir, h.OutputDir),
		Logger:                    h.Logger,
		Timeout:                   30 * time.Second,
	})

	runner := gorun.New(&gorun.Config{
		ExecProgramPath:      "./" + compiler.MainOutputFileNameWithExtension(),
		RunArguments:         h.ArgumentsToRunServer,
		ExitChan:             h.ExitChan,
		Logger:               h.Logger,
		KillAllOnStop:        true,
		DisableGlobalCleanup: h.Config.DisableGlobalCleanup,
		WorkingDir:           filepath.Join(h.AppRootDir, h.OutputDir),
	})

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
	defer func() {
		if wg != nil {
			wg.Done()
		}
	}()
	return s.startServer()
}

func (s *externalStrategy) startServer() error {
	e := errors.New("startServer")

	// ALWAYS COMPILE before running
	err := s.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(e, err)
	}

	// RUN
	err = s.goRun.RunProgram()
	if err != nil {
		return errors.Join(e, err)
	}

	s.handler.Logger("Started:", path.Join(s.handler.SourceDir, s.handler.mainFileExternalServer), "Port:", s.handler.AppPort)
	return nil
}

func (s *externalStrategy) Stop() error {
	if s.goRun != nil {
		s.handler.Logger("Stopping external server...")
		return s.goRun.StopProgramAndCleanup(true)
	}
	return nil
}

func (s *externalStrategy) Restart() error {
	s.handler.Logger("Restarting External Server...")
	err := s.Stop()
	if err != nil {
		return err
	}
	waitForPortFree(s.handler.AppPort) // Ensure port is free
	return s.Start(nil)
}

func (s *externalStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
	if event == "write" {
		s.handler.Logger("Go file modified, restarting external server ...")
		err := s.Restart()
		if err != nil {
			s.handler.Logger("RestartServer failed:", err)
		} else {
			s.handler.Logger("RestartServer succeeded")
		}
		return err
	}
	return nil
}
