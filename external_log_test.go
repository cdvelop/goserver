package server

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestExternalStrategyLogsAreVisible verifies that logs from gobuild and gorun
// are properly forwarded to the ServerHandler's logger even when SetLog is called
// after the strategy is created (which is the normal flow when using TUI).
func TestExternalStrategyLogsAreVisible(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory and a server file that will produce a compile error
	sourceDir := "src"
	outputDir := "bin"
	fullSourcePath := filepath.Join(tmpDir, sourceDir)
	if err := os.MkdirAll(fullSourcePath, 0755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, outputDir), 0755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	// Create handler WITHOUT Logger in config (simulates app/section-build.go)
	cfg := &Config{
		AppRootDir:           tmpDir,
		SourceDir:            sourceDir,
		OutputDir:            outputDir,
		MainInputFile:        "main.go",
		AppPort:              "19090",
		ExitChan:             make(chan bool),
		DisableGlobalCleanup: true,
	}
	h := New(cfg)

	// Create a server file with a syntax error to trigger a build failure
	// We do this AFTER New() to ensure we start in Internal mode, so
	// SetExternalServerMode(true) actually triggers a switch and Start().
	serverFile := filepath.Join(fullSourcePath, "main.go")
	badCode := `package main

import "fmt"

func main() {
	fmt.Println("hello"  // Missing closing parenthesis - syntax error
}
`
	if err := os.WriteFile(serverFile, []byte(badCode), 0644); err != nil {
		t.Fatalf("writing server file: %v", err)
	}

	// Capture logs AFTER handler creation (simulates TUI calling SetLog later)
	var logs []string
	var logsMu sync.Mutex
	h.SetLog(func(msgs ...any) {
		logsMu.Lock()
		defer logsMu.Unlock()
		for _, m := range msgs {
			logs = append(logs, strings.TrimSpace(m.(string)))
		}
	})

	// Force external mode and start
	if err := h.SetExternalServerMode(true); err != nil {
		// Expected: compilation should fail
		t.Logf("SetExternalServerMode returned error (expected): %v", err)
	}

	// Wait for logs to appear (polling) instead of fixed sleep
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		logsMu.Lock()
		allLogs := strings.Join(logs, "\n")
		logsMu.Unlock()

		if strings.Contains(allLogs, "build failed") || strings.Contains(allLogs, "syntax error") {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !found {
		t.Log("Timed out waiting for logs")
	}

	// Close exit channel to cleanup
	close(cfg.ExitChan)

	// Check that the compilation error was logged
	logsMu.Lock()
	defer logsMu.Unlock()

	allLogs := strings.Join(logs, "\n")
	t.Logf("Captured logs:\n%s", allLogs)

	// The error should mention something about the syntax error (missing parenthesis)
	if !strings.Contains(allLogs, "syntax") && !strings.Contains(allLogs, "error") && !strings.Contains(allLogs, "build failed") {
		t.Errorf("Expected compilation error to be visible in logs, but got:\n%s", allLogs)
	}
}

// TestExternalStrategyRuntimeErrorLogsAreVisible verifies that runtime errors
// from the external server process are properly forwarded to the logger.
func TestExternalStrategyRuntimeErrorLogsAreVisible(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory and a server file that will compile but fail at runtime
	sourceDir := "src"
	outputDir := "bin"
	fullSourcePath := filepath.Join(tmpDir, sourceDir)
	if err := os.MkdirAll(fullSourcePath, 0755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, outputDir), 0755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	// Create handler WITHOUT Logger in config (simulates app/section-build.go)
	cfg := &Config{
		AppRootDir:           tmpDir,
		SourceDir:            sourceDir,
		OutputDir:            outputDir,
		MainInputFile:        "main.go",
		AppPort:              "19091",
		ExitChan:             make(chan bool),
		DisableGlobalCleanup: true,
	}
	h := New(cfg)

	// Create a server file that compiles but prints an error and exits
	// We do this AFTER New() to ensure we start in Internal mode
	serverFile := filepath.Join(fullSourcePath, "main.go")
	failingCode := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "RUNTIME_ERROR: initialization failed")
	os.Exit(1)
}
`
	if err := os.WriteFile(serverFile, []byte(failingCode), 0644); err != nil {
		t.Fatalf("writing server file: %v", err)
	}

	// Capture logs AFTER handler creation (simulates TUI calling SetLog later)
	var logs []string
	var logsMu sync.Mutex
	h.SetLog(func(msgs ...any) {
		logsMu.Lock()
		defer logsMu.Unlock()
		for _, m := range msgs {
			if s, ok := m.(string); ok {
				logs = append(logs, strings.TrimSpace(s))
			}
		}
	})

	// Switch to external mode - this should compile successfully
	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("SetExternalServerMode failed: %v", err)
	}

	// Wait for logs to appear (polling) instead of fixed sleep
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		logsMu.Lock()
		allLogs := strings.Join(logs, "\n")
		logsMu.Unlock()

		if strings.Contains(allLogs, "RUNTIME_ERROR") {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !found {
		t.Log("Timed out waiting for logs")
	}

	// Close exit channel to cleanup
	close(cfg.ExitChan)
	time.Sleep(100 * time.Millisecond)

	// Check that the runtime error was logged
	logsMu.Lock()
	defer logsMu.Unlock()

	allLogs := strings.Join(logs, "\n")
	t.Logf("Captured logs:\n%s", allLogs)

	// The runtime error message should be visible
	if !strings.Contains(allLogs, "RUNTIME_ERROR") {
		t.Errorf("Expected runtime error 'RUNTIME_ERROR' to be visible in logs, but got:\n%s", allLogs)
	}
}
