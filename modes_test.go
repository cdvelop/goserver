package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockStore struct {
	data map[string]string
}

func (m *mockStore) Get(key string) (string, error) { return m.data[key], nil }
func (m *mockStore) Set(key, value string) error    { m.data[key] = value; return nil }

func TestServerHandler_Value(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"
	h := New(cfg)

	// Case 1: Default (Internal + In-Memory)
	if got := h.Value(); got != "Execution External:F, Build OnDisk:F" {
		t.Errorf("default Value() = %q", got)
	}

	// Case 2: Modified state
	h.executionInternal = false
	h.compilationOnDisk = true
	if got := h.Value(); got != "Execution External:T, Build OnDisk:T" {
		t.Errorf("modified Value() = %q", got)
	}
}

func TestServerHandler_Change(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantExec bool // true = internal, false = external
		wantDisk bool
	}{
		{"both T", "Execution External:T, Build OnDisk:T", false, true},
		{"both F", "Execution External:F, Build OnDisk:F", true, false},
		{"lowercase", "Execution External:t, Build OnDisk:f", false, false},
		{"full words", "Execution External:true, Build OnDisk:false", false, false},
		{"uppercase words", "Execution External:TRUE, Build OnDisk:FALSE", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh handler for each subtest to avoid race conditions
			tmpData := t.TempDir()
			cfg := NewConfig()
			cfg.AppRootDir = tmpData
			cfg.SourceDir = "src"
			h := New(cfg)
			db := &mockStore{data: make(map[string]string)}
			h.Store = db

			h.Change(tt.input)

			if h.executionInternal != tt.wantExec {
				t.Errorf("executionInternal = %v, want %v", h.executionInternal, tt.wantExec)
			}
			if h.compilationOnDisk != tt.wantDisk {
				t.Errorf("compilationOnDisk = %v, want %v", h.compilationOnDisk, tt.wantDisk)
			}
		})
	}
}

func TestSetExternalServerMode_SwitchesToExternal(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"
	cfg.OutputDir = "bin"

	h := New(cfg)

	if !h.executionInternal {
		t.Fatal("expected initial executionInternal to be true")
	}

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	if h.executionInternal {
		t.Fatal("expected executionInternal to be false after switching to external")
	}

	// Verify file was generated
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatalf("expected server file to be generated at %s", targetPath)
	}
}

func TestSetExternalServerMode_SwitchesToInternal_Unmodified(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	if err := h.SetExternalServerMode(false); err != nil {
		t.Fatalf("expected no error switching back to internal when unmodified, got: %v", err)
	}

	if !h.executionInternal {
		t.Fatal("expected executionInternal to be true after switching back to internal")
	}
}

func TestSetExternalServerMode_BlocksInternal_WhenModified(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	// Modify generated file
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	err := os.WriteFile(targetPath, []byte("package main\n\nfunc main() { /* modified */ }\n"), 0644)
	if err != nil {
		t.Fatalf("failed to modify server file: %v", err)
	}

	err = h.SetExternalServerMode(false)
	if err == nil {
		t.Fatal("expected error when switching to internal after modification, but got nil")
	}

	if !strings.Contains(err.Error(), "customized") {
		t.Errorf("expected error message to mention 'customized', got: %v", err)
	}

	if h.executionInternal {
		t.Fatal("expected executionInternal to remain false after failed switch")
	}
}

func TestSetCompilationOnDisk(t *testing.T) {
	h := New(nil)

	if h.compilationOnDisk {
		t.Fatal("expected default compilationOnDisk to be false")
	}

	h.SetCompilationOnDisk(true)
	if !h.compilationOnDisk {
		t.Fatal("expected compilationOnDisk to be true after setting")
	}
}

func TestModeSwitchWhileRunning_DoesNotHang(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"
	cfg.OutputDir = "bin"
	cfg.AppPort = "18080" // Use non-standard port to avoid conflicts
	cfg.ExitChan = make(chan bool)
	cfg.DisableGlobalCleanup = true

	var logs []string
	var logsMu sync.Mutex
	cfg.Logger = func(msgs ...any) {
		logsMu.Lock()
		defer logsMu.Unlock()
		for _, m := range msgs {
			logs = append(logs, fmt.Sprintf("%v", m))
		}
	}

	h := New(cfg)
	db := &mockStore{data: make(map[string]string)}
	h.Store = db

	// Start the internal server in a goroutine (like app does)
	var wg sync.WaitGroup
	wg.Add(1)
	serverStarted := make(chan struct{})
	go func() {
		close(serverStarted)
		h.StartServer(&wg)
	}()
	<-serverStarted

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Call Change() to switch to external mode - this is what app does via TUI
	done := make(chan struct{})
	go func() {
		h.Change("Execution External:T, Build OnDisk:F")
		close(done)
	}()

	select {
	case <-done:
		// Change completed
		t.Log("Change() completed")
	case <-time.After(5 * time.Second):
		t.Fatal("Change() hung - mode switch blocked")
	}

	// Check logs for errors
	logsMu.Lock()
	for _, log := range logs {
		t.Logf("LOG: %s", log)
	}
	logsMu.Unlock()

	// Verify mode changed
	if h.executionInternal {
		t.Error("expected executionInternal to be false after Change")
	}

	// Cleanup - signal exit
	close(cfg.ExitChan)

	// Wait for server goroutine to finish (with timeout)
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Log("Server shutdown cleanly")
	case <-time.After(3 * time.Second):
		t.Log("Server shutdown timed out (expected in some cases)")
	}
}
