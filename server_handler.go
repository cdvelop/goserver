package goserver

import (
	"errors"
	"fmt"
	"time"
)

func (h *ServerHandler) RestartServer() error {
	var this = errors.New("restart server")

	// STOP current server
	err := h.goRun.StopProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "StopProgram failed: %v\n", err)
		return errors.Join(this, errors.New("stop server"), err)
	}

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	fmt.Fprintln(h.Logger, "Compiling server...")
	err = h.goCompiler.CompileProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "CompileProgram failed: %v\n", err)
		return errors.Join(this, errors.New("compile server"), err)
	}
	fmt.Fprintln(h.Logger, "CompileProgram succeeded")

	// RUN new version
	err = h.goRun.RunProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "RunProgram failed: %v\n", err)
		return errors.Join(this, errors.New("run server"), err)
	}
	fmt.Fprintln(h.Logger, "RunProgram succeeded")

	fmt.Fprintln(h.Logger, "RestartServer completed successfully")
	return nil
}
