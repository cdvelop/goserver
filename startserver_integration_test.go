//go:build integration
// +build integration

package goserver_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gs "github.com/cdvelop/goserver"
)

// Integration test: black-box verification that StartServer generates the external
// server file (if missing), starts the external server and that it responds on /health.
// Uses only the public API of the package. Skipped by default.
func TestStartServerRunsGeneratedServerAndResponds(t *testing.T) {
	// enabled: run automatically

	tmp := t.TempDir()

	// find a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("getting free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	var logBuf bytes.Buffer
	logger := func(messages ...any) {
		fmt.Fprintln(&logBuf, messages...)
	}

	// create public folder and a simple index file that the generated server should serve
	publicDir := filepath.Join(tmp, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("creating public folder: %v", err)
	}
	indexPath := filepath.Join(publicDir, "index.html")
	const indexContent = "INDEX_OK"
	if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}

	sourceDir := "src/app"
	outputDir := "deploy"
	fullSourcePath := filepath.Join(tmp, sourceDir)
	if err := os.MkdirAll(fullSourcePath, 0755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}

	cfg := &gs.Config{
		AppRootDir: tmp,
		SourceDir:  sourceDir,
		OutputDir:  outputDir,
		AppPort:    fmt.Sprintf("%d", port),
		Logger:     logger,
		ExitChan:   make(chan bool, 1),
		ArgumentsToRunServer: func() []string {
			// Pass the public directory to the server as an environment variable
			return []string{fmt.Sprintf("PUBLIC_DIR=%s", publicDir)}
		},
	}

	h := gs.New(cfg)

	// Ensure external file absent
	target := h.MainInputFileRelativePath()
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("expected no external server file at %s", target)
	}

	// Start server in background
	var wg sync.WaitGroup
	wg.Add(1)
	go h.StartServer(&wg)

	// Wait for StartServer to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// StartServer returned (expected)
	case <-time.After(20 * time.Second):
		t.Fatalf("StartServer did not complete within timeout; logs: %s", logBuf.String())
	}

	// Poll /health until we get 200 or timeout
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// success: also verify that root (/) serves our static index file
				// request the explicit index file to avoid depending on directory index behavior
				resp2, err2 := client.Get(fmt.Sprintf("http://127.0.0.1:%d/index.html", port))
				if err2 == nil {
					b, _ := io.ReadAll(resp2.Body)
					resp2.Body.Close()
					if !bytes.Contains(b, []byte(indexContent)) {
						t.Fatalf("root did not serve index content; got: %q", string(b))
					}
				} else {
					t.Fatalf("error requesting root file: %v", err2)
				}

				// success: signal server to exit via ExitChan
				select {
				case cfg.ExitChan <- true:
				default:
				}
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("generated server did not respond on /health within timeout; logs: %s", logBuf.String())
}
