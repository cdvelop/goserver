package goserver

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cdvelop/gobuild"
	"github.com/cdvelop/gorun"
)

type ServerHandler struct {
	*Config
	mainFileExternalServer string // eg: main.server.go
	goCompiler             *gobuild.GoBuild
	goRun                  *gorun.GoRun
}

type Config struct {
	AppRootDir                  string               // e.g., /home/user/project (application root directory)
	SourceDir                   string               // directory location of main.go e.g., src/cmd/appserver (relative to AppRootDir)
	OutputDir                   string               // compilation and execution directory e.g., deploy/appserver (relative to AppRootDir)
	ArgumentsForCompilingServer func() []string      // e.g., []string{"-X 'main.version=v1.0.0'"}
	ArgumentsToRunServer        func() []string      // e.g., []string{"dev"}
	AppPort                     string               // e.g., 8080
	Logger                      func(message ...any) // For logging output
	ExitChan                    chan bool            // Global channel to signal shutdown
}

// NewConfig provides a default configuration.
func NewConfig() *Config {
	return &Config{
		AppRootDir: ".",
		SourceDir:  "src/cmd/appserver",
		OutputDir:  "deploy/appserver",
		AppPort:    "8080",
		Logger: func(message ...any) {
			// Silent by default
		},
		ExitChan: make(chan bool),
	}
}

func New(c *Config) *ServerHandler {
	// Ensure the output directory exists
	if err := os.MkdirAll(filepath.Join(c.AppRootDir, c.OutputDir), 0755); err != nil {
		if c.Logger != nil {
			c.Logger("Error creating output directory:", err)
		}
	}
	var exe_ext = ""
	if runtime.GOOS == "windows" {
		exe_ext = ".exe"
	}

	sh := &ServerHandler{
		Config:                 c,
		mainFileExternalServer: "main.go", // Convention: main file is always main.go
	}

	sh.goCompiler = gobuild.New(&gobuild.Config{
		Command:                   "go",
		MainInputFileRelativePath: filepath.Join(c.AppRootDir, c.SourceDir, sh.mainFileExternalServer),
		OutName:                   "main", // Convention: output is always main
		Extension:                 exe_ext,
		CompilingArguments:        c.ArgumentsForCompilingServer,
		OutFolderRelativePath:     filepath.Join(c.AppRootDir, c.OutputDir),
		Logger:                    c.Logger,
		Timeout:                   30 * time.Second,
	})

	sh.goRun = gorun.New(&gorun.Config{
		ExecProgramPath: "./" + sh.goCompiler.MainOutputFileNameWithExtension(),
		RunArguments:    c.ArgumentsToRunServer,
		ExitChan:        c.ExitChan,
		Logger:          c.Logger,
		KillAllOnStop:   true,
		WorkingDir:      filepath.Join(c.AppRootDir, c.OutputDir), // Execute from OutputDir
	})

	return sh
}

// MainInputFileRelativePath returns the path relative to AppRootDir (e.g., "src/cmd/appserver/main.go")
func (h *ServerHandler) MainInputFileRelativePath() string {
	return filepath.Join(h.SourceDir, h.mainFileExternalServer)
}

func (h *ServerHandler) SupportedExtensions() []string {
	return []string{".go"}
}

// UnobservedFiles returns the list of files that should not be tracked by file watchers eg: main.exe, main_temp.exe
func (h *ServerHandler) UnobservedFiles() []string {
	return h.goCompiler.UnobservedFiles()
}