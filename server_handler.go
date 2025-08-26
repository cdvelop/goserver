package goserver

import (
	"errors"
	"time"
)

func (h *ServerHandler) StartExternalServer() error {
	this := errors.New("StartExternalServer")

	// ALWAYS COMPILE before running to ensure latest changes
	err := h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(this, err)
	}

	// RUN
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, err)
	}

	return nil
}

func (h *ServerHandler) RestartExternalServer() error {
	var this = errors.New("restart external server")

	// STOP current server
	err := h.goRun.StopProgram()
	if err != nil {
		return errors.Join(this, errors.New("StopProgram"), err)
	}

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	err = h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(this, errors.New("CompileProgram"), err)
	}

	// RUN new version
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, errors.New("RunProgram"), err)
	}

	return nil
}

// RestartServer reinicia el servidor externo y devuelve un mensaje de estado
func (h *ServerHandler) RestartServer() (string, error) {
	err := h.RestartExternalServer()
	if err != nil {
		return "Error restarting external server", err
	}
	return "External server restarted", nil
}
