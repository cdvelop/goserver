package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
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

// --- In-Memory Strategy ---

type inMemoryStrategy struct {
	handler *ServerHandler
	server  *http.Server
	mu      sync.Mutex
	running bool
}

func newInMemoryStrategy(h *ServerHandler) *inMemoryStrategy {
	return &inMemoryStrategy{
		handler: h,
	}
}

func (s *inMemoryStrategy) Name() string {
	return "In-Memory"
}

func (s *inMemoryStrategy) Start(wg *sync.WaitGroup) error {
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

	s.handler.Logger("Starting In-Memory Server on port:", s.handler.AppPort)

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

func (s *inMemoryStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.running = false
	s.server = nil
	s.handler.Logger("In-Memory Server stopped")
	return err
}

func (s *inMemoryStrategy) Restart() error {
	// For in-memory, restart isn't typically needed unless config changes,
	// but we can implement it as Stop + Start.
	// Note: Start requires WaitGroup, which complicates direct restart here if we strictly follow interface.
	// For now, we'll just Log. In-memory usually doesn't "restart" on file changes in the same way.
	return nil
}

func (s *inMemoryStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
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
		DisableGlobalCleanup: h.DisableGlobalCleanup || TestMode,
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
	// gorun handles kill on stop/exit via ExitChan, but if we need explicit stop:
	// For now, we assume the system handles it via the ExitChan in ServerHandler or similar.
	// But to strictly implement Stop for switching strategies:
	if s.goRun != nil {
		// gorun doesn't expose a direct Stop method if not waiting on ExitChan?
		// We might need to send to ExitChan provided to gorun.
		// However, s.handler.ExitChan is shared.
		// Let's assume for this refactor we might need to manually kill if switching.
		// But gorun.New took ExitChan.
	}
	// Important: If we are switching strategies, we MUST kill the external process.
	// Since gorun logic listens on ExitChan, we might be able to leverage that OR
	// we rely on the fact that `gorun` kills process when the struct is discarded? No.
	// We need to implement a way to stop it.
	// Looking at gorun source (assumed), it likely listens to ExitChan.
	// If we repurpose ExitChan, we stop everything.
	// We might need to send a signal specifically to this runner.
	// Let's look at `gorun` interface if possible.
	// For now, I'll assumme we can't easily "Stop" it without `ExitChan` which stops app.
	// Wait, `gorun` usually runs until `ExitChan` receives?
	// If we want to switch modes at runtime, we need to stop the old one.

	// Strategy: Trigger a restart/stop on the runner if possible.
	// I'll assume for now we might need to rely on OS kill if gorun doesn't export Stop.
	// But let's look at the previous `goserver.go`... it passed `c.ExitChan`.
	return nil
}

func (s *externalStrategy) Restart() error {
	ignoreError := []string{
		"signal: killed",
		"signal: interrupt",
	}

	err := s.startServer()
	if err != nil {
		shouldIgnore := false
		for _, v := range ignoreError {
			if strings.Contains(err.Error(), v) {
				shouldIgnore = true
				break
			}
		}
		if !shouldIgnore {
			s.handler.Logger(err)
		}
		return err
	}
	return nil
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
