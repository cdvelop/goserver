package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

// TestMode is a global flag to indicate the server is running in a test environment.
// This is used to disable aggressive cleanups and other disruptive behaviors.
var TestMode bool

type ServerHandler struct {
	*Config
	mainFileExternalServer string // eg: main.server.go
	strategy               ServerStrategy
	executionInternal      bool // true = embedded server (internal), false = external process
	compilationOnDisk      bool // true = artifacts to disk, false = in-memory
	onLog                  func(message ...any)
}

type Config struct {
	AppRootDir                  string                 // e.g., /home/user/project (application root directory)
	SourceDir                   string                 // directory location of main.go e.g., src/cmd/appserver (relative to AppRootDir)
	OutputDir                   string                 // compilation and execution directory e.g., deploy/appserver (relative to AppRootDir)
	PublicDir                   string                 // default public dir for generated server (e.g., src/web/public)
	MainInputFile               string                 // main input file name (default: "main.go", can be "server.go", etc.)
	ArgumentsForCompilingServer func() []string        // e.g., []string{"-X 'main.version=v1.0.0'"}
	ArgumentsToRunServer        func() []string        // e.g., []string{"dev"}
	AppPort                     string                 // e.g., 8080
	Routes                      []func(*http.ServeMux) // Functions to register routes on the HTTP server
	DisableGlobalCleanup        bool                   // If true, disables global cleanup in gorun during restarts
	Logger                      func(message ...any)   // Logger function
	ExitChan                    chan bool              // Global channel to signal shutdown
}

// NewConfig provides a default configuration.
func NewConfig() *Config {
	return &Config{
		AppRootDir:    ".",
		SourceDir:     "web",
		OutputDir:     "web",
		PublicDir:     "web/public",
		MainInputFile: "main.go", // Default convention
		AppPort:       "8080",
		Routes:        nil,
		Logger:        nil,
		ExitChan:      make(chan bool),
	}
}

func New(c *Config) *ServerHandler {
	// Accept nil and fill missing fields with defaults to avoid panics when caller
	// provides a partially populated Config.
	dc := NewConfig() // default config

	if c == nil {
		c = dc
	} else {
		// Fill zero-value fields with defaults to be defensive
		if c.AppRootDir == "" {
			c.AppRootDir = dc.AppRootDir
		}
		if c.SourceDir == "" {
			c.SourceDir = dc.SourceDir
		}
		if c.OutputDir == "" {
			c.OutputDir = dc.OutputDir
		}
		if c.PublicDir == "" {
			c.PublicDir = dc.PublicDir
		}
		if c.MainInputFile == "" {
			c.MainInputFile = dc.MainInputFile
		}
		if c.AppPort == "" {
			c.AppPort = dc.AppPort
		}
		if c.ExitChan == nil {
			c.ExitChan = make(chan bool)
		}
		if c.ArgumentsForCompilingServer == nil {
			c.ArgumentsForCompilingServer = func() []string { return nil }
		}
		if c.ArgumentsToRunServer == nil {
			c.ArgumentsToRunServer = func() []string { return nil }
		}
	}

	sh := &ServerHandler{
		Config:                 c,
		mainFileExternalServer: c.MainInputFile, // Use configured file name
		onLog:                  c.Logger,
	}

	// Default to Internal Execution Mode
	sh.executionInternal = true
	sh.strategy = newInternalStrategy(sh)
	// sh.log("Server initialized in Internal Mode (default)")

	return sh
}

func (h *ServerHandler) Name() string {
	return "SERVER"
}

func (h *ServerHandler) SetLog(f func(message ...any)) {
	h.onLog = f
}

func (h *ServerHandler) log(messages ...any) {
	if h.onLog != nil {
		h.onLog(messages...)
	}
}

// MainInputFileRelativePath returns the path relative to AppRootDir (e.g., "src/cmd/appserver/main.go")
func (h *ServerHandler) MainInputFileRelativePath() string {
	return filepath.Join(h.SourceDir, h.mainFileExternalServer)
}

func (h *ServerHandler) SupportedExtensions() []string {
	return []string{".go"}
}

// UnobservedFiles returns the list of files that should not be tracked by file watchers
func (h *ServerHandler) UnobservedFiles() []string {
	if !h.executionInternal {
		if ext, ok := h.strategy.(*externalStrategy); ok {
			return ext.goCompiler.UnobservedFiles()
		}
	}
	return []string{}
}

// serverFileWasModified returns true if the external server file
// has been modified from the original template
func (h *ServerHandler) serverFileWasModified() bool {
	targetPath := filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)

	currentContent, err := os.ReadFile(targetPath)
	if err != nil {
		return false // File doesn't exist, not modified from some original (it's missing)
	}

	expectedContent, err := h.getExpectedServerContent()
	if err != nil {
		return true // Can't generate expectedContent, assume modified/customized
	}

	return string(currentContent) != expectedContent
}

// SetCompilationOnDisk sets whether the server artifacts should be written to disk.
func (h *ServerHandler) SetCompilationOnDisk(onDisk bool) {
	h.compilationOnDisk = onDisk
	// If we are in external mode, it will compile to disk on Start/Restart
	if !h.executionInternal {
		h.log("Server Compilation mode set to:", map[bool]string{true: "OnDisk", false: "InMemory"}[onDisk])
	}
}

// SetExternalServerMode switches between Internal and External server strategies.
// When switching to External, it also:
// 1. Generates server template files if they don't exist
// 2. Compiles the server
// 3. Starts the external process
func (h *ServerHandler) SetExternalServerMode(external bool) error {
	if external {
		if h.executionInternal {
			h.log("Switching to External Server Mode...")

			// Generate template files if they don't exist
			if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
				return err
			}

			// Stop current internal strategy
			if err := h.strategy.Stop(); err != nil {
				h.log("Warning stopping internal server:", err)
			}

			waitForPortFree(h.AppPort)

			h.executionInternal = false
			h.strategy = newExternalStrategy(h)

			if err := h.strategy.Start(nil); err != nil {
				return err
			}
		}
	} else {
		if !h.executionInternal {
			// Check if server file was modified before allowing switch back to internal
			if h.serverFileWasModified() {
				return errors.New("cannot switch to Internal execution mode: server file has been customized")
			}

			h.log("Switching to Internal Server Mode...")

			if err := h.strategy.Stop(); err != nil {
				h.log("Warning stopping external server:", err)
			}

			waitForPortFree(h.AppPort)

			h.executionInternal = true
			h.strategy = newInternalStrategy(h)

			go h.strategy.Start(nil)
		}
	}
	return nil
}
