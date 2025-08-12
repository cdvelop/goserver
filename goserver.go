package goserver

import (
	"io"
	"net/http"
	"path"
	"runtime"
	"time"

	"github.com/cdvelop/gobuild"
	"github.com/cdvelop/gorun"
)

type ServerHandler struct {
	*Config
	mainFileExternalServer string // eg: main.server.go
	internalServerRun      bool
	server                 *http.Server
	goCompiler             *gobuild.GoBuild
	goRun                  *gorun.GoRun
}

type Config struct {
	RootFolder                  string          // eg: web
	MainFileWithoutExtension    string          // eg: main.server
	ArgumentsForCompilingServer func() []string // eg: []string{"-X 'main.version=v1.0.0'"}
	ArgumentsToRunServer        func() []string // eg: []string{"dev" }
	PublicFolder                string          // eg: public
	AppPort                     string          // eg : 8080
	Writer                      io.Writer       // For logging output
	ExitChan                    chan bool       // Canal global para señalizar el cierre
}

func New(c *Config) *ServerHandler {

	var exe_ext = ""
	if runtime.GOOS == "windows" {
		exe_ext = ".exe"
	}

	sh := &ServerHandler{
		Config:                 c,
		mainFileExternalServer: c.MainFileWithoutExtension + ".go",
		internalServerRun:      false,
		server:                 nil,
	}
	sh.goCompiler = gobuild.New(&gobuild.Config{
		Command:            "go",
		MainFilePath:       path.Join(c.RootFolder, sh.mainFileExternalServer),
		OutName:            c.MainFileWithoutExtension,
		Extension:          exe_ext,
		CompilingArguments: c.ArgumentsForCompilingServer,
		OutFolder:          c.RootFolder,
		Writer:             c.Writer,
		Timeout:            30 * time.Second,
	})
	sh.goRun = gorun.New(&gorun.GoRunConfig{
		ExecProgramPath: path.Join(c.RootFolder, sh.goCompiler.MainOutputFileNameWithExtension()),
		RunArguments:    c.ArgumentsToRunServer,
		ExitChan:        c.ExitChan,
		Writer:          c.Writer,
	})

	return sh
}

// MainFilePath eg: <root>/web/main.server.go
func (h *ServerHandler) MainFilePath() string {
	return h.goCompiler.MainFilePath()

} // Name returns "GoServer"
func (h *ServerHandler) Name() string {
	return "GoServer"
}

// UnobservedFiles returns the list of files that should not be tracked by file watchers eg: main.exe, main_temp.exe
func (h *ServerHandler) UnobservedFiles() []string {
	return h.goCompiler.UnobservedFiles()
}
