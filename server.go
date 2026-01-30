package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/tinywasm/fmt"
)

// Store defines the minimal interface for persistent storage
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// UI defines the minimal interface for UI interaction
type UI interface {
	RefreshUI()
}

const (
	StoreKeyExternalServer    = "server_external_mode"
	StoreKeyCompilationOnDisk = "server_compilation_ondisk"
	EnvKeyServerPort          = "SERVER_PORT"
	EnvKeyServerHttps         = "SERVER_HTTPS"
)

// TestMode is a global flag to indicate the server is running in a test environment.
// This is used to disable aggressive cleanups and other disruptive behaviors.
var TestMode bool

type noopStore struct{}

func (noopStore) Get(string) (string, error) { return "", nil }
func (noopStore) Set(string, string) error   { return nil }

type noopUI struct{}

func (noopUI) RefreshUI() {}

type ServerHandler struct {
	*Config
	mainFileExternalServer string // eg: main.server.go
	strategy               ServerStrategy
	strategyMu             sync.RWMutex // protects strategy field
	executionInternal      bool         // true = embedded server (internal), false = external process
	compilationOnDisk      bool         // true = artifacts to disk, false = in-memory
	onLog                  func(message ...any)
	openBrowserOnce        sync.Once
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
	Https                       bool                   // true if HTTPS is enabled
	Routes                      []func(*http.ServeMux) // Functions to register routes on the HTTP server
	DisableGlobalCleanup        bool                   // If true, disables global cleanup in gorun during restarts
	Logger                      func(message ...any)   // Logger function
	ExitChan                    chan bool              // Global channel to signal shutdown
	OpenBrowser                 func(port string, https bool)
	Store                       Store                    // Persistent storage for modes
	UI                          UI                       // UI for refresh notifications
	OnExternalModeExecution     func(isExternal bool)    // Called before StartServer to notify mode change
	GitIgnoreAdd                func(entry string) error // Callback to add entries to .gitignore
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
		if c.OnExternalModeExecution == nil {
			c.OnExternalModeExecution = func(bool) {}
		}
		if c.GitIgnoreAdd == nil {
			c.GitIgnoreAdd = func(string) error { return nil }
		}
	}

	sh := &ServerHandler{
		Config:                 c,
		mainFileExternalServer: c.MainInputFile, // Use configured file name
		onLog:                  c.Logger,
	}

	if sh.Store == nil {
		sh.Store = noopStore{}
	}
	if sh.UI == nil {
		sh.UI = noopUI{}
	}

	// Default to Internal Execution Mode
	sh.executionInternal = true
	sh.strategy = newInternalStrategy(sh)

	// Load persisted states
	if val, err := sh.Store.Get(StoreKeyExternalServer); err == nil && val == "true" {
		sh.executionInternal = false
		sh.strategy = newExternalStrategy(sh)
	}

	// If a custom server file exists, it takes precedence over default configuration
	serverFilePath := filepath.Join(sh.AppRootDir, sh.SourceDir, sh.mainFileExternalServer)
	if _, err := os.Stat(serverFilePath); err == nil && sh.executionInternal {
		// Only log if we are actually switching mode due to file presence
		if sh.onLog != nil {
			sh.onLog("Found existing server file, switching to External Server Mode")
		}
		sh.executionInternal = false
		sh.strategy = newExternalStrategy(sh)
		// Update store to reflect the state
		_ = sh.Store.Set(StoreKeyExternalServer, "true")
	}

	if val, err := sh.Store.Get(StoreKeyCompilationOnDisk); err == nil && val == "true" {
		sh.compilationOnDisk = true
	}

	return sh
}

// Label implements HandlerEdit.Label
func (h *ServerHandler) Label() string {
	return "Server Modes"
}

// Value implements HandlerEdit.Value
func (h *ServerHandler) Value() string {
	exec := "F"
	if !h.executionInternal {
		exec = "T"
	}
	return "Execution External:" + exec
}

// Change implements HandlerEdit.Change
func (h *ServerHandler) Change(newValue string) {
	pairs := fmt.Convert(newValue).Split(",")

	for _, pair := range pairs {
		// e.g., pair = "Execution External:T" or " Build OnDisk:F"
		s := fmt.Convert(pair).TrimSpace().String()
		pos := fmt.Index(s, ":")
		if pos == -1 {
			continue
		}

		key := fmt.Convert(s[:pos]).TrimSpace().String()
		val := fmt.Convert(s[pos+1:]).TrimSpace().ToLower().String()
		isTrue := val == "t" || val == "true"

		switch key {
		case "Execution External":
			if err := h.SetExternalServerMode(isTrue); err != nil {
				h.log("Mode change error:", err)
			}
		}
	}

	h.RefreshUI()
}

func (h *ServerHandler) RefreshUI() {
	if h.UI != nil {
		h.UI.RefreshUI()
	}
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

// Restart restarts the server.
// It delegates to the current strategy's Restart method.
func (h *ServerHandler) Restart() error {
	h.strategyMu.RLock()
	defer h.strategyMu.RUnlock()

	if h.strategy != nil {
		h.log("Restarting server strategy:", h.strategy.Name())
		return h.strategy.Restart()
	}
	return nil
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
	_ = h.Store.Set(StoreKeyCompilationOnDisk, map[bool]string{true: "true", false: "false"}[onDisk])

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
	h.strategyMu.Lock()
	defer h.strategyMu.Unlock()

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

			go h.strategy.Start(nil)
			_ = h.Store.Set(StoreKeyExternalServer, "true")
		}
	} else {
		if !h.executionInternal {
			return errors.New("cannot switch back to Internal execution mode once External mode is active")
		}
	}
	return nil
}
