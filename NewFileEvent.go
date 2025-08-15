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

			// Stop internal server if running to avoid port conflicts
			if h.internalServerRun {
				if err := h.StopInternalServer(); err != nil {
					return fmt.Errorf("stopping internal server: %w", err)
				}
			}

			// Restart external server with new changes
			return h.RestartExternalServer()
		}

		// Case 2: Shared Go file was modified
		if !h.internalServerRun {
			if h.Logger != nil {
				fmt.Fprintln(h.Logger, "Shared Go file modified, restarting external server ...")
			}
			return h.RestartExternalServer()
		}
	}

	// Case 3: External server file was created for first time
	if event == "create" && fileName == h.mainFileExternalServer {
		if h.Logger != nil {
			fmt.Fprintln(h.Logger, "New external server detected")
		}

		// Stop internal server if running
		if h.internalServerRun {
			if err := h.StopInternalServer(); err != nil {
				return fmt.Errorf("stopping internal server: %w", err)
			}
		}

		// Start the new external server
		return h.StartExternalServer()
	}

	return nil
}
