package server

import (
	"net"
	"sync"
	"time"
)

// StartServer initiates the server using the current strategy (In-Memory or External)
func (h *ServerHandler) StartServer(wg *sync.WaitGroup) {
	if err := h.strategy.Start(wg); err != nil {
		h.Logger("StartServer error:", err)
	}
}

// StopServer stops the server and waits for the port to be released.
func (h *ServerHandler) StopServer() error {
	h.Logger("Stopping server...")
	err := h.strategy.Stop()
	if err != nil {
		h.Logger("StopServer error:", err)
	}

	// Wait for port to be released (up to 5 seconds)
	addr := ":" + h.AppPort
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			h.Logger("Warning: Port", h.AppPort, "still seems occupied after timeout")
			return err
		case <-ticker.C:
			ln, dialErr := net.Listen("tcp", addr)
			if dialErr == nil {
				ln.Close()
				h.Logger("Port", h.AppPort, "is now free")
				return err
			}
		}
	}
}
