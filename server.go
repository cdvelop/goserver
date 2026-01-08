package server

import (
	"net/http"
	"path/filepath"
)

// TestMode is a global flag to indicate the server is running in a test environment.
// This is used to disable aggressive cleanups and other disruptive behaviors.
var TestMode bool

type ServerHandler struct {
	*Config
	mainFileExternalServer string // eg: main.server.go
	strategy               ServerStrategy
	inMemory               bool // true if running internal server, false if external process
	buildOnDisk            bool // true if compilation artifacts should be written to disk
	log                    func(message ...any)
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
	}

	// Default to In-Memory Strategy (Internal Server)
	sh.inMemory = true
	sh.strategy = newInMemoryStrategy(sh)
	// sh.Logger("Server initialized in In-Memory Mode (default)")

	return sh
}

func (h *ServerHandler) Name() string {
	return "SERVER"
}

func (h *ServerHandler) SetLog(f func(message ...any)) {
	h.log = f
}

func (h *ServerHandler) Logger(messages ...any) {
	if h.log != nil {
		h.log(messages...)
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
	if !h.inMemory {
		if ext, ok := h.strategy.(*externalStrategy); ok {
			return ext.goCompiler.UnobservedFiles()
		}
	}
	return []string{}
}

// SetBuildOnDisk sets whether the server artifacts should be written to disk.
func (h *ServerHandler) SetBuildOnDisk(onDisk bool) {
	h.buildOnDisk = onDisk
	// If we are in external mode, it will compile to disk on Start/Restart
	if !h.inMemory {
		h.Logger("Server BuildOnDisk set to:", onDisk)
	}
}

// SetExternalServerMode switches between Internal and External server strategies.
func (h *ServerHandler) SetExternalServerMode(external bool) {
	if external {
		if h.inMemory {
			h.Logger("Switching to External Server Mode...")
			h.inMemory = false
			h.strategy.Stop()
			h.strategy = newExternalStrategy(h)
			h.strategy.Start(nil)
		}
	} else {
		if !h.inMemory {
			h.Logger("Switching to Internal Server Mode...")
			h.inMemory = true
			h.strategy.Stop()
			h.strategy = newInMemoryStrategy(h)
			h.strategy.Start(nil)
		}
	}
}
