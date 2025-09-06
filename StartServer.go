package goserver

import (
	"errors"
	"os"
	"path"
	"sync"
)

// Start inicia el servidor como goroutine
func (h *ServerHandler) StartServer(wg *sync.WaitGroup) {
	defer wg.Done()

	if _, err := os.Stat(path.Join(h.RootFolder, h.mainFileExternalServer)); os.IsNotExist(err) {
		// If external server file doesn't exist, generate it from embedded markdown
		if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
			h.Logger("generate server from markdown:", err)
		}
	}

	// build and run server
	err := h.startServer()
	if err != nil {
		h.Logger("starting server:", err)
	}
}

// private server start
func (h *ServerHandler) startServer() error {
	this := errors.New("start server")

	// ALWAYS COMPILE before running to ensure latest changes
	err := h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(this, err)
	}

	// RUN
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, err)
	}

	return nil
}
