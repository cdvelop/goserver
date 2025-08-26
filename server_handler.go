package goserver

import (
	"errors"
	"fmt"
	"time"
)

func (h *ServerHandler) RestartServer() error {
	var this = errors.New("restart server")

	fmt.Fprintln(h.Logger, "DEBUG: RestartServer starting...")

	// STOP current server
	fmt.Fprintln(h.Logger, "DEBUG: Stopping current server...")
	err := h.goRun.StopProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "DEBUG: StopProgram failed: %v\n", err)
		return errors.Join(this, errors.New("stop server"), err)
	}
	fmt.Fprintln(h.Logger, "DEBUG: StopProgram succeeded")

	// Wait a brief moment to ensure cleanup is complete
	// This prevents issues where the previous process hasn't fully released resources
	time.Sleep(100 * time.Millisecond)

	// COMPILE latest changes
	fmt.Fprintln(h.Logger, "DEBUG: Compiling server...")
	err = h.goCompiler.CompileProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "DEBUG: CompileProgram failed: %v\n", err)
		return errors.Join(this, errors.New("compile server"), err)
	}
	fmt.Fprintln(h.Logger, "DEBUG: CompileProgram succeeded")

	// RUN new version
	fmt.Fprintln(h.Logger, "DEBUG: Running new server...")
	err = h.goRun.RunProgram()
	if err != nil {
		fmt.Fprintf(h.Logger, "DEBUG: RunProgram failed: %v\n", err)
		return errors.Join(this, errors.New("run server"), err)
	}
	fmt.Fprintln(h.Logger, "DEBUG: RunProgram succeeded")

	fmt.Fprintln(h.Logger, "DEBUG: RestartServer completed successfully")
	return nil
}
