package goserver

import (
	"io"
	"path"
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
	AppRootDir                  string          // e.g., /home/user/project (application root directory)
	RootFolder                  string          // e.g., web (relative to AppRootDir or absolute path)
	MainFileWithoutExtension    string          // e.g., main.server
	ArgumentsForCompilingServer func() []string // e.g., []string{"-X 'main.version=v1.0.0'"}
	ArgumentsToRunServer        func() []string // e.g., []string{"dev"}
	PublicFolder                string          // e.g., public
	AppPort                     string          // e.g., 8080
	Logger                      io.Writer       // For logging output
	ExitChan                    chan bool       // Global channel to signal shutdown
}

func New(c *Config) *ServerHandler {

	var exe_ext = ""
	if runtime.GOOS == "windows" {
		exe_ext = ".exe"
	}

	// Ensure logger is safe for concurrent writes
	c.Logger = ensureSyncWriter(c.Logger)

	sh := &ServerHandler{
		Config:                 c,
		mainFileExternalServer: c.MainFileWithoutExtension + ".go",
	}
	sh.goCompiler = gobuild.New(&gobuild.Config{
		Command:                   "go",
		MainInputFileRelativePath: sh.mainFileExternalServer, // Use just the filename since OutFolderRelativePath is the target directory
		OutName:                   c.MainFileWithoutExtension,
		Extension:                 exe_ext,
		CompilingArguments:        c.ArgumentsForCompilingServer,
		OutFolderRelativePath:     c.RootFolder,
		Logger:                    c.Logger,
		Timeout:                   30 * time.Second,
	})
	sh.goRun = gorun.New(&gorun.Config{
		ExecProgramPath: "./" + sh.goCompiler.MainOutputFileNameWithExtension(), // Use ./filename to avoid PATH search
		RunArguments:    c.ArgumentsToRunServer,
		ExitChan:        c.ExitChan,
		Logger:          c.Logger,
		KillAllOnStop:   true,         // Kill all instances when stopping to prevent orphaned processes
		WorkingDir:      c.RootFolder, // Execute from the folder containing the binary
	})

	return sh
}

// MainInputFileRelativePath returns the path relative to AppRootDir (e.g., "pwa/main.server.go")
func (h *ServerHandler) MainInputFileRelativePath() string {
	// Calculate relative path from AppRootDir to the server file
	if h.AppRootDir != "" {
		// If RootFolder is absolute, make it relative to AppRootDir
		relativeRootFolder := h.RootFolder
		if filepath.IsAbs(h.RootFolder) {
			if rel, err := filepath.Rel(h.AppRootDir, h.RootFolder); err == nil {
				relativeRootFolder = rel
			}
		}
		return filepath.Join(relativeRootFolder, h.mainFileExternalServer)
	}

	// Fallback to the old behavior if AppRootDir is not set
	return path.Join(h.RootFolder, h.mainFileExternalServer)
}

func (h *ServerHandler) SupportedExtensions() []string {
	return []string{".go"}
}

// UnobservedFiles returns the list of files that should not be tracked by file watchers eg: main.exe, main_temp.exe
func (h *ServerHandler) UnobservedFiles() []string {
	return h.goCompiler.UnobservedFiles()
}
