package goserver

import "fmt"

// event: create,write,remove,rename
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error {
	if event == "write" {
		// Case 1: External server file was modified
		if fileName == h.mainFileExternalServer {
			if h.Logger != nil {
				fmt.Fprintln(h.Logger, "External server modified, restarting ...")
			}
			// Restart external server with new changes
			return h.RestartExternalServer()
		}

		// Case 2: Any Go file was modified - restart external server
		if h.Logger != nil {
			fmt.Fprintln(h.Logger, "Go file modified, restarting external server ...")
		}
		return h.RestartExternalServer()
	}

	// Case 3: External server file was created for first time
	if event == "create" && fileName == h.mainFileExternalServer {
		if h.Logger != nil {
			fmt.Fprintln(h.Logger, "New external server detected")
		}
		// Start the new external server
		return h.StartExternalServer()
	}

	return nil
}
