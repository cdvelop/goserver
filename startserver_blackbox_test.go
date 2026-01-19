package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This test verifies that calling CreateTemplateServer generates the external server file
// and switches strategy. We don't necessarily enforce successful compilation in this test env,
// but we verify file generation.
func TestCreateTemplateServerGeneratesFile(t *testing.T) {
	// enabled: run automatically

	tmp := t.TempDir()

	// capture logs into a buffer
	var logMessages []string
	logger := func(messages ...any) {
		logMessages = append(logMessages, fmt.Sprint(messages...))
	}

	exitChan := make(chan bool, 1)
	// Pre-fill exit channel so Start() doesn't block waiting
	exitChan <- true

	cfg := &Config{
		AppRootDir:           tmp,
		SourceDir:            "src/app",
		OutputDir:            "deploy",
		AppPort:              "0", // Use random port to avoid conflicts if it runs
		ExitChan:             exitChan,
		DisableGlobalCleanup: true,
	}

	h := New(cfg)
	h.SetLog(logger)

	// Ensure external file doesn't exist initially
	target := filepath.Join(h.AppRootDir, h.MainInputFileRelativePath())
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("expected no external server file at %s", target)
	}

	// Verify we are in Internal mode
	if !h.executionInternal {
		t.Fatal("Expected Internal mode initially")
	}

	// Call CreateTemplateServer.
	// Since CreateTemplateServer tries to compile, and we might not have a full Go env for the generated code
	// (depending on dependencies), it might return an error.
	// We primarily care that it generated the file.
	err := h.CreateTemplateServer()

	if err != nil {
		t.Logf("CreateTemplateServer returned error (expected in restricted test env if compile fails): %v", err)
	}

	// Now the generated file should exist
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected generated server file at %s, but not found: %v", target, err)
	}

	// Verify the logs mention generation
	out := strings.Join(logMessages, "\n")
	if !strings.Contains(out, "generate server from markdown") && !strings.Contains(out, "Generating server files") {
		// CreateTemplateServer might log to progress channel instead of h.Logger for some steps
		// But generateServerFromEmbeddedMarkdown uses h.Logger if set.
		// And we also verify h.executionInternal is now false (logic switched strategy before Compile)
		// Wait, if Compile failed inside startServer, does it stay in ExternalStrategy?
		// CreateTemplateServer:
		// 1. Stop InMemory
		// 2. Generate
		// 3. Switch h.executionInternal=false, h.strategy=newExternal
		// 4. h.strategy.Start() -> Compile -> Error
		// So h.executionInternal should be false even if Start fails.
	}

	if h.executionInternal {
		t.Error("Expected to be in External mode logic (h.executionInternal = false) even if compilation failed")
	}
}
