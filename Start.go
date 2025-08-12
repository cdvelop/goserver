package goserver

import (
	"fmt"
	"os"
	"path"
	"sync"
)

// Start inicia el servidor como goroutine
func (h *ServerHandler) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Fprintln(h.Writer, "Server Start ...")
	// fmt.Println("Server Start ...")

	if _, err := os.Stat(path.Join(h.RootFolder, h.mainFileExternalServer)); os.IsNotExist(err) {
		// ejecutar el servidor interno de archivos estáticos
		h.StartInternalServerFiles()
	} else {
		// construir y ejecutar el servidor externo
		err := h.StartExternalServer()
		if err != nil {
			fmt.Fprintln(h.Writer, "starting external server:", err)
		}
	}
}
