package goserver

import (
	"errors"
	"os"
	"path"
	"strings"
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

		ignoreError := []string{
			"signal: killed",    // initial compile may fail if file is being created
			"signal: interrupt", // compile error
		}

		// print the error only when none of the ignored patterns match
		shouldIgnore := false
		for _, v := range ignoreError {
			if strings.Contains(err.Error(), v) {
				shouldIgnore = true
				break
			}
		}
		if !shouldIgnore {
			h.Logger(err)
		}
	}

}

// private server start
func (h *ServerHandler) startServer() error {

	e := errors.New("startServer")

	// ALWAYS COMPILE before running to ensure latest changes
	err := h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(e, err)
	}

	// RUN
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(e, err)
	}

	h.Logger("Started:", h.RootFolder+"/"+h.mainFileExternalServer, "Port:", h.AppPort)

	return nil
}
