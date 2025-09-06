package goserver

import (
	"errors"
	"time"
)

func (h *ServerHandler) RestartServer() error {
	var this = errors.New("restart server")

	// STOP current server
	err := h.goRun.StopProgram()
	if err != nil {
		h.Logger("StopProgram failed:", err)
		return errors.Join(this, errors.New("stop server"), err)
	}

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	h.Logger("Compiling server...")
	err = h.goCompiler.CompileProgram()
	if err != nil {
		h.Logger("CompileProgram failed:", err)
		return errors.Join(this, errors.New("compile server"), err)
	}
	h.Logger("CompileProgram succeeded")

	// RUN new version
	err = h.goRun.RunProgram()
	if err != nil {
		h.Logger("RunProgram failed:", err)
		return errors.Join(this, errors.New("run server"), err)
	}
	h.Logger("RunProgram succeeded")

	h.Logger("RestartServer completed successfully")
	return nil
}
