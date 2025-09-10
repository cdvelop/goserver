package goserver

import (
	"errors"
	"time"
)

func (h *ServerHandler) RestartServer() error {
	var e = errors.New("RestartServer")

	// STOP current server
	err := h.goRun.StopProgram()
	if err != nil {
		h.Logger(e, "StopProgram failed:", err)
		return errors.Join(e, errors.New("stop server"), err)
	}

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	// h.Logger("Compiling server...")
	err = h.goCompiler.CompileProgram()
	if err != nil {
		h.Logger(e, "CompileProgram failed:", err)
		return errors.Join(e, errors.New("compile server"), err)
	}
	// h.Logger("CompileProgram succeeded")

	// RUN new version
	err = h.goRun.RunProgram()
	if err != nil {
		h.Logger(e, "RunProgram failed:", err)
		return errors.Join(e, errors.New("run server"), err)
	}
	// h.Logger("RunProgram succeeded")

	h.Logger("Rebooted")
	return nil
}
