package server

import "errors"

// CreateTemplateServer switches from Internal to External mode.
// It generates the server files (if not present), compiles, and runs them.
// This implements the transition from "Internal" to "External" mode.
func (h *ServerHandler) CreateTemplateServer() error {
	h.strategyMu.Lock()

	if !h.executionInternal {
		h.strategyMu.Unlock()
		h.log("Server is already in external mode.")
		return nil
	}

	h.log("Stopping Internal Server...")
	// Stop the current internal server
	if err := h.strategy.Stop(); err != nil {
		return errors.Join(errors.New("failed to stop internal server"), err)
	}

	h.log("Generating server files...")
	// Generate the physical files for the server
	if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
		return errors.Join(errors.New("failed to generate server files"), err)
	}

	h.log("Switching to External Process Strategy...")
	// Switch strategy state
	h.executionInternal = false
	h.strategy = newExternalStrategy(h)

	// Capture strategy to call Start on it without needing the lock
	s := h.strategy
	h.strategyMu.Unlock()

	h.log("Starting External Server...")
	// Start the new external server (compiles and runs)
	// We pass nil for wg because this is a runtime transition, not application startup
	if err := s.Start(nil); err != nil {
		// If start fails, should we try to revert?
		// For now, return the error.
		return errors.Join(errors.New("failed to start external server"), err)
	}

	h.log("Server successfully switched to External mode.")
	return nil
}
