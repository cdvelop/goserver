package server

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StartServer initiates the server using the current strategy (In-Memory or External)
func (h *ServerHandler) StartServer(wg *sync.WaitGroup) {
	serverFilePath := filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)

	h.strategyMu.Lock()
	if _, err := os.Stat(serverFilePath); err == nil && h.executionInternal {
		h.log("Found existing server file, switching to External Server Mode")
		h.executionInternal = false
		h.strategy = newExternalStrategy(h)
		if h.Store != nil {
			_ = h.Store.Set(StoreKeyExternalServer, "true")
		}
	}
	isInternal := h.executionInternal
	strategy := h.strategy
	h.strategyMu.Unlock()

	if !isInternal {
		if err := h.BeforeExternalServerStart(); err != nil {
			h.log("BeforeExternalServerStart failed, aborting transition:", err)
			if wg != nil {
				wg.Done()
			}
			return
		}
	}

	if err := strategy.Start(wg); err != nil {
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

// Restart restarts the server.
// It delegates to the current strategy's Restart method.
func (h *ServerHandler) Restart() error {
	h.strategyMu.RLock()
	defer h.strategyMu.RUnlock()

	if h.strategy != nil {
		h.log("Restarting server strategy:", h.strategy.Name())
		return h.strategy.Restart()
	}
	return nil
}
