package goserver

import "fmt"

// event: create,write,remove,rename
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error {

	fmt.Fprintf(h.Logger, "File event: %s, %s, %s, %s\n", fileName, extension, filePath, event)

	if event == "write" {
		// Case 1: External server file was modified

		fmt.Fprintln(h.Logger, "Go file modified, restarting external server ...")
		return h.RestartServer()
	}

	// Case 2: External server file was created for first time
	if event == "create" && fileName == h.mainFileExternalServer {
		fmt.Fprintln(h.Logger, "New external server detected")
		// Start the new external server
		return h.startServer()
	}

	return nil
}
