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
		return errors.Join(this, errors.New("stop server"), err)
	}

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	err = h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(this, errors.New("compile server"), err)
	}

	// RUN new version
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, errors.New("run server"), err)
	}

	return nil
}
