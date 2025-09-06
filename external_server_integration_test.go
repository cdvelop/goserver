//go:build integration
// +build integration

package goserver_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

	cfg := &Config{
		RootFolder:                  tmp,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     fmt.Sprintf("%d", port),
		Logger:                      func(messages ...any) { fmt.Fprintln(os.Stdout, messages...) },
		ExitChan:                    make(chan bool),
	}

	h := New(cfg)

	// generate the external server file
	if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// build the generated server
	binPath := filepath.Join(tmp, "main.server")
	build := exec.Command("go", "build", "-o", binPath, h.mainFileExternalServer)
	build.Dir = tmp
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	// start the binary with a context so we can timeout the whole run
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Dir = tmp
	// ensure the process sees PORT env in case the generated server prefers it
	cmd.Env = append(os.Environ(), "PORT="+cfg.AppPort)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting server binary: %v", err)
	}
	// ensure we kill the process at the end
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
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
