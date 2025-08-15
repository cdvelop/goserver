package goserver

import (
	"fmt"
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
			fmt.Fprintln(h.Logger, "generate server from markdown:", err)
		}
	}

	// build and run external server
	err := h.StartExternalServer()
	if err != nil {
		fmt.Fprintln(h.Logger, "starting external server:", err)
	}
}
