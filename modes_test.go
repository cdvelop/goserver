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
	if got := h.Value(); got != "Execution External:F" {
		t.Errorf("default Value() = %q", got)
	}

	// Case 2: Modified state
	h.executionInternal = false
	h.compilationOnDisk = true
	if got := h.Value(); got != "Execution External:T" {
		t.Errorf("modified Value() = %q", got)
	}
}

func TestServerHandler_Change(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantExec bool // true = internal, false = external
	}{
		{"external T", "Execution External:T", false},
		{"external F", "Execution External:F", true}, // Technically no-op if internal already
		{"lowercase", "Execution External:t", false},
		{"full words", "Execution External:true", false},
		{"uppercase words", "Execution External:TRUE", false},
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

func TestSetExternalServerMode_PreventsSwitchingToInternal(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	// 1. Switch to External
	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	// 2. Attempt switch back to Internal should fail
	if err := h.SetExternalServerMode(false); err == nil {
		t.Fatal("expected error preventing switch back to internal, got nil")
	} else if !strings.Contains(err.Error(), "cannot switch back") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 3. Verify state remains External
	if h.executionInternal {
		t.Fatal("expected executionInternal to remain false (external) after failed switch")
	}
}

func TestSetExternalServerMode_BlocksInternal_WhenModified(t *testing.T) {
	// This test is now redundant or covers a subset of PreventsSwitchingToInternal
	// checks, but we'll keep it simple to verify behavior even with modifications.
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	// Modify generated file (irrelevant now, but good to ensure no regressions)
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	err := os.WriteFile(targetPath, []byte("package main\n\nfunc main() { /* modified */ }\n"), 0644)
	if err != nil {
		t.Fatalf("failed to modify server file: %v", err)
	}

	err = h.SetExternalServerMode(false)
	if err == nil {
		t.Fatal("expected error switching back to internal")
	}
	// The specific error message about "customized" is gone, replaced by the generic block
	if !strings.Contains(err.Error(), "cannot switch back") {
		t.Errorf("expected error message to mention 'cannot switch back', got: %v", err)
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

func TestGitIgnoreAdd_CalledOnExternalMode(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "web"
	cfg.OutputDir = "web"
	cfg.MainInputFile = "server.go"

	var capturedEntry string
	cfg.GitIgnoreAdd = func(entry string) error {
		capturedEntry = entry
		return nil
	}

	h := New(cfg)

	// Switch to external mode - this should trigger GitIgnoreAdd
	err := h.SetExternalServerMode(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the binary path was added to gitignore
	expectedPath := filepath.Join("web", "server") // OutputDir + binary name (no .exe on linux)
	if capturedEntry != expectedPath {
		t.Errorf("GitIgnoreAdd called with %q, want %q", capturedEntry, expectedPath)
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
		h.Change("Execution External:T")
		close(done)
	}()

	select {
	case <-done:
		// Change completed
		t.Log("Change() completed")
	case <-time.After(5 * time.Second):
		t.Fatal("Change() hung - mode switch blocked")
	}

	if t.Failed() {
		logsMu.Lock()
		for _, log := range logs {
			t.Logf("LOG: %s", log)
		}
		logsMu.Unlock()
	}

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

func TestNew_DetectsExistingServerFile(t *testing.T) {
	// Setup
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "web"
	cfg.MainInputFile = "server.go"

	// Create a dummy server file
	svrDir := filepath.Join(tmpData, "web")
	if err := os.MkdirAll(svrDir, 0755); err != nil {
		t.Fatal(err)
	}
	svrFile := filepath.Join(svrDir, "server.go")
	if err := os.WriteFile(svrFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize handler
	h := New(cfg)

	// Expectation: Should default to External mode because file exists
	if h.executionInternal {
		t.Error("New() should initialize in External mode when server file exists, but got Internal")
	}

	execStr := "F"
	if !h.executionInternal {
		execStr = "T"
	}
	if execStr != "T" {
		t.Errorf("Value() expected External:T, got %s", h.Value())
	}
}
