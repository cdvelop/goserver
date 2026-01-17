//go:build integration
// +build integration

package server_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinywasm/server"
)

// Test that the generated external server can be built and responds on /health.
// This is a slow integration test and is skipped by default.
func TestGeneratedServerStartsAndResponds(t *testing.T) {
	t.Skip("integration test - enable manually")

	tmp := t.TempDir()

	// prepare public folder
	public := filepath.Join(tmp, "public")
	if err := os.MkdirAll(public, 0755); err != nil {
		t.Fatalf("creating public folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(public, "index.html"), []byte("ok"), 0644); err != nil {
		t.Fatalf("creating index: %v", err)
	}

	// pick a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	sourceDir := filepath.Join(tmp, "src", "app")
	outputDir := filepath.Join(tmp, "deploy")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("creating source directory: %v", err)
	}

	cfg := &server.Config{
		AppRootDir: tmp,
		SourceDir:  filepath.ToSlash(strings.TrimPrefix(sourceDir, tmp+string(os.PathSeparator))),
		OutputDir:  filepath.ToSlash(strings.TrimPrefix(outputDir, tmp+string(os.PathSeparator))),
		AppPort:    fmt.Sprintf("%d", port),
		ExitChan:   make(chan bool),
	}

	h := server.New(cfg)
	h.SetLog(func(messages ...any) { fmt.Fprintln(os.Stdout, messages...) })

	// Start server (uses internal API but from server_test package we need to export it or use New and switch)
	// Actually, this test wants to manually build and run the server file.
	// We need to call GenerateServer (exported) if it exists.
	// But generateServerFromEmbeddedMarkdown is unexported.

	// generate the external server file
	// Since we are in server_test, we can only call exported methods.
	// If it's not exported, we might need to skip this or export it.
	// For now, let's see if there is an exported way.
	// Actually, StartServer already generates it if missing.

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("failed to set external server mode: %v", err)
	}
	h.StartServer(nil) // This should generate and start

	// ensure we kill the process at the end via StopServer
	t.Cleanup(func() {
		h.StopServer()
	})

	// poll /health until success or timeout
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://127.0.0.1:" + fmt.Sprintf("%d", port) + "/health"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server did not respond on /health within timeout")
}
