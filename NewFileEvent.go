package goserver

// event: create,write,remove,rename
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error {

	h.Logger("File event:", fileName, extension, filePath, event)

	if event == "write" {
		// Case 1: External server file was modified

		h.Logger("Go file modified, restarting external server ...")
		err := h.RestartServer()
		if err != nil {
			h.Logger("RestartServer failed:", err)
		} else {
			h.Logger("RestartServer succeeded")
		}
		return err
	}

	// Case 2: External server file was created for first time
	if event == "create" && fileName == h.mainFileExternalServer {
		h.Logger("New external server detected")
		// Start the new server
		return h.startServer()
	}

	return nil
}
