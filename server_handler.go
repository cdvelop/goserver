package goserver

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
)

func (h *ServerHandler) StartExternalServer() error {
	this := errors.New("StartExternalServer")

	// COMPILE
	// Check if executable exists
	if _, err := os.Stat(h.goRun.ExecProgramPath); os.IsNotExist(err) {
		// COMPILE only if executable doesn't exist
		err := h.goCompiler.CompileProgram()
		if err != nil {
			return errors.Join(this, err)
		}
	}

	// RUN
	err := h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, err)
	}

	return nil
}

func (h *ServerHandler) StartInternalServerFiles() {
	// Crear el servidor de archivos estáticos

	publicFolder := path.Join(h.RootFolder, h.PublicFolder)

	fs := http.FileServer(http.Dir(publicFolder))

	// Configurar el servidor HTTP
	h.server = &http.Server{
		Addr:    ":" + h.AppPort,
		Handler: fs,
	}

	fmt.Fprintln(h.Writer, "Godev Server Files:", publicFolder, "Running port:", h.AppPort)
	// Iniciar el servidor en una goroutine
	h.internalServerRun = true

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(h.Writer, "Internal Server Files error:", err)
		}
	}()
}

func (h *ServerHandler) StopInternalServer() error {
	if h.server != nil {
		h.internalServerRun = false
		// fmt.Fprintln(h.Writer,"Internal Server Stop")
		return h.server.Close()
	}
	return nil
}

func (h *ServerHandler) RestartInternalServer() error {
	err := h.StopInternalServer()
	if err != nil {
		return err
	}

	h.StartInternalServerFiles()
	return nil
}

func (h *ServerHandler) RestartExternalServer() error {
	var this = errors.New("Restart External Server")

	// STOP
	err := h.goRun.StopProgram()
	if err != nil {
		return errors.Join(this, errors.New("StopProgram"), err)

	}

	// COMPILE
	err = h.goCompiler.CompileProgram()
	if err != nil {
		return errors.Join(this, errors.New("CompileProgram"), err)
	}

	// RUN
	err = h.goRun.RunProgram()
	if err != nil {
		return errors.Join(this, errors.New("RunProgram"), err)
	}

	return nil
}

// RestartServer reinicia el servidor actual (interno o externo) y devuelve un mensaje de estado
func (h *ServerHandler) RestartServer() (string, error) {
	if h.internalServerRun {
		err := h.RestartInternalServer()
		if err != nil {
			return "Error restarting internal server", err
		}
		return "Internal server restarted", nil
	} else {
		err := h.RestartExternalServer()
		if err != nil {
			return "Error restarting external server", err
		}
		return "External server restarted", nil
	}
}
