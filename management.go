package server

import (
	"net"
	"sync"
	"time"
)

// StartServer initiates the server using the current strategy (In-Memory or External)
func (h *ServerHandler) StartServer(wg *sync.WaitGroup) {
	// Notify external systems before starting (e.g., switch client/assets to disk mode)
	h.OnExternalModeExecution(!h.executionInternal)

	if err := h.strategy.Start(wg); err != nil {
		h.log("StartServer error:", err)
	}
}

// StopServer stops the server and waits for the port to be released.
func (h *ServerHandler) StopServer() error {
	h.log("Stopping server...")
	err := h.strategy.Stop()
	if err != nil {
		h.log("StopServer error:", err)
	}

	// Wait for port to be released (up to 5 seconds)
	addr := ":" + h.AppPort
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			h.log("Warning: Port", h.AppPort, "still seems occupied after timeout")
			return err
		case <-ticker.C:
			ln, dialErr := net.Listen("tcp", addr)
			if dialErr == nil {
				ln.Close()
				h.log("Port", h.AppPort, "is now free")
				return err
			}
		}
	}
}

func (h *ServerHandler) RestartServer() error {
	return h.strategy.Restart()
}
