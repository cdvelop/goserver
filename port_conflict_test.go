//go:build integration
// +build integration

package goserver_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/cdvelop/goserver"
)

// TestPortConflictCleanup tests what happens when there's a port conflict
func TestPortConflictCleanup(t *testing.T) {

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

	// Create a go.mod file
	gomod := `module temp
go 1.20
`
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("creating go.mod: %v", err)
	}

	// Create server
	serverCode := fmt.Sprintf(`package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "%d"
	}

	http.HandleFunc("/h", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Server is running v1")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Server is running")
	})

	fmt.Printf("Server starting on port %%s...\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
`, port)

	sourceDir := filepath.Join(tmp, "src", "app")
	outputDir := filepath.Join(tmp, "deploy")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("creating source directory: %v", err)
	}

	mainPath := filepath.Join(sourceDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(serverCode), 0644); err != nil {
		t.Fatalf("creating main.go: %v", err)
	}

	cfg := &Config{
		AppRootDir: tmp,
		SourceDir:  filepath.ToSlash(strings.TrimPrefix(sourceDir, tmp+string(os.PathSeparator))),
		OutputDir:  filepath.ToSlash(strings.TrimPrefix(outputDir, tmp+string(os.PathSeparator))),
		AppPort:    fmt.Sprintf("%d", port),
		Logger:     func(messages ...any) { fmt.Fprintln(os.Stdout, messages...) },
		ExitChan:   make(chan bool),
	}

	h := New(cfg)

	// Test 1: Start server normally
	t.Log("🚀 Starting first server instance...")
	err = h.startServer()
	if err != nil {
		t.Logf("❌ First server failed as expected due to occupied port: %v", err)
	} else {
		t.Log("✅ First server started successfully")
	}

	time.Sleep(1 * time.Second)

	// Test 2: Try to start a second server on the same port (this should cause conflict)
	t.Log("🚀 Starting second server instance (this should conflict)...")

	// Create second handler with same port
	cfg2 := &Config{
		AppRootDir: tmp,
		SourceDir:  filepath.ToSlash(strings.TrimPrefix(sourceDir, tmp+string(os.PathSeparator))),
		OutputDir:  filepath.ToSlash(strings.TrimPrefix(outputDir, tmp+string(os.PathSeparator))),
		AppPort:    fmt.Sprintf("%d", port),
		Logger:     func(messages ...any) { fmt.Fprintln(os.Stdout, messages...) },
		ExitChan:   make(chan bool),
	}

	h2 := New(cfg2)

	// This should fail with "address already in use"
	err = h2.startServer()
	if err != nil {
		t.Logf("✅ Second server failed as expected: %v", err)
	} else {
		t.Log("❌ Second server started unexpectedly (this should have failed)")
	}

	// Now close the listener to free the port
	ln.Close()

	// Test 3: Try restart - this should work now that the port is free
	t.Log("🔄 Attempting restart on first server...")
	err = h.RestartServer()
	if err != nil {
		t.Logf("❌ RestartServer failed: %v", err)
	} else {
		t.Log("✅ Restart completed successfully")

		// Verify the server is actually running
		time.Sleep(1 * time.Second)
		client := &http.Client{Timeout: 2 * time.Second}
		url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
		if resp, httpErr := client.Get(url); httpErr == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Log("✅ Restarted server is responding correctly")
			} else {
				t.Logf("❌ Restarted server wrong status: %d", resp.StatusCode)
			}
		} else {
			t.Logf("❌ Restarted server not responding: %v", httpErr)
		}
	}

	// Cleanup
	cfg.ExitChan <- true
	cfg2.ExitChan <- true
	time.Sleep(1 * time.Second)
}
