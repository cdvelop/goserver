package goserver

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// This is a black-box test: it uses only the public API of the package under test.
// It verifies that calling StartServer will generate the external server file
// when it doesn't exist. The test is intentionally lightweight and does not
// attempt to compile or run the generated binary.
func TestStartServerGeneratesExternalFile(t *testing.T) {
	// enabled: run automatically

	tmp := t.TempDir()

	// capture logs into a buffer so we can inspect what StartServer printed
	var logMessages []string
	logger := func(messages ...any) {
		logMessages = append(logMessages, fmt.Sprint(messages...))
	}

	cfg := &Config{
		RootFolder:                  tmp,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     "9090",
		Logger:                      logger,
		ExitChan:                    make(chan bool),
	}

	h := New(cfg)

	// Ensure external file doesn't exist initially
	// h.MainInputFileRelativePath already returns an absolute path using RootFolder
	target := h.MainInputFileRelativePath()
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("expected no external server file at %s", target)
	}

	// Start the server in background. StartServer expects a WaitGroup.
	var wg sync.WaitGroup
	wg.Add(1)
	go h.StartServer(&wg)

	// Wait for StartServer to finish
	wg.Wait()

	// Now the generated file should exist
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected generated server file at %s, but not found: %v", target, err)
	}

	// Verify the logs mention generation (best-effort, since generate logs on success)
	out := strings.Join(logMessages, "\n")
	if !strings.Contains(out, "Generated server file") && !strings.Contains(out, "generate server from markdown") {
		t.Logf("log output did not contain generation messages: %q", out)
	}
}
