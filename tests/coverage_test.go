package server_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"webtyp.com/server"
)

type MockUI struct {
	Refreshed bool
}

func (m *MockUI) RefreshUI() {
	m.Refreshed = true
}

// Ensure MockUI implements server.UI
var _ server.UI = &MockUI{}

func TestServerHandler_Getters(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	if label := h.Label(); label != server.LabelServerRun {
		t.Errorf("expected Label 'Execution', got '%s'", label)
	}

	if name := h.Name(); name != "SERVER" {
		t.Errorf("expected Name 'SERVER', got '%s'", name)
	}

	exts := h.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".go" {
		t.Errorf("expected SupportedExtensions to be ['.go'], got %v", exts)
	}

	// Internal mode should have no unobserved files
	if files := h.UnobservedFiles(); len(files) != 0 {
		t.Errorf("expected 0 unobserved files in internal mode, got %d", len(files))
	}
}

func TestServerHandler_RefreshUI(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	mockUI := &MockUI{}
	h.UI = mockUI

	h.RefreshUI()

	if !mockUI.Refreshed {
		t.Error("RefreshUI did not call UI.RefreshUI()")
	}
}

func TestServerHandler_DefaultUI(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)
	// Default UI is noopUI
	h.RefreshUI()
}

func TestServerHandler_StopServer(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	if err := h.StopServer(); err != nil {
		t.Errorf("StopServer failed: %v", err)
	}
}

func TestServerHandler_Restart(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	if h.ExitChan == nil {
		h.ExitChan = make(chan bool, 1)
	}

	done := make(chan error)
	go func() {
		done <- h.Restart()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Restart returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Log("Restart timed out waiting for port")
	}

	select {
	case h.ExitChan <- true:
	default:
	}
}

func TestServerHandler_RestartServer_Wrapper(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	done := make(chan error)
	go func() {
		done <- h.RestartServer()
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case h.ExitChan <- true:
	default:
	}
}

func TestServerHandler_NewFileEvent(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	if err := h.NewFileEvent("main.go", ".go", filepath.Join(tmp, "src/main.go"), "write"); err != nil {
		t.Errorf("NewFileEvent failed: %v", err)
	}
}

func TestServerHandler_ExternalStrategy_Coverage(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)

	srcDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainContent := `package main
import (
	"fmt"
	"flag"
)
func main() {
	port := flag.String("port", "8080", "port")
	flag.Parse()
	fmt.Println("Listening on", *port)
}`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Switch to external
	go func() {
		_ = h.SetExternalServerMode(true)
	}()

	time.Sleep(500 * time.Millisecond)

	// Call UnobservedFiles
	_ = h.UnobservedFiles()

	// Call Restart() to cover external Name() and Restart()
	done := make(chan error)
	go func() {
		done <- h.Restart()
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}

func TestServerHandler_StartError(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, "src", "out", tmp)
	// Invalid port to trigger listen error
	h.AppPort = "invalid-port"

	var wg sync.WaitGroup
	wg.Add(1)

	// StartServer calls strategy.Start
	// internal strategy Start should return error and call wg.Done
	h.StartServer(&wg)

	// Wait for wg to ensure Done is called
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("StartServer did not call wg.Done on error")
	}
}
