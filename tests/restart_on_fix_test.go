package server_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"webtyp.com/server"
)

// Test that when a write produces a compilation error, fixing the file and
// triggering another write causes the external server to be recompiled and run.
func TestNewFileEvent_RestartsAfterFix(t *testing.T) {
	tmp := t.TempDir()

	var mu sync.Mutex
	var logMessages []string
	logger := func(messages ...any) {
		mu.Lock()
		logMessages = append(logMessages, fmt.Sprint(messages...))
		mu.Unlock()
	}

	// Define source and output directories
	sourceDir := filepath.Join(tmp, "src", "app")
	outputDir := filepath.Join(tmp, "deploy")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("creating source directory: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("creating output directory: %v", err)
	}

	// find a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("getting free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	exitChan := make(chan bool, 1)

	serverFile := filepath.Join(sourceDir, "main.go")

	// Initial correct content so the server starts
	initialContent := fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
    "log"
)

func main() {
    fmt.Println("VERSION_OK")
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "VERSION_OK")
    })
    log.Fatal(http.ListenAndServe(":%d", nil))
}`, port)

	if err := os.WriteFile(serverFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("writing initial server file: %v", err)
	}

	handler := server.New()
	handler.SetAppRootDir(tmp)
	handler.SetSourceDir(filepath.ToSlash(strings.TrimPrefix(sourceDir, tmp+string(os.PathSeparator))))
	handler.SetOutputDir(filepath.ToSlash(strings.TrimPrefix(outputDir, tmp+string(os.PathSeparator))))
	handler.SetPort(fmt.Sprintf("%d", port))
	handler.SetExitChan(exitChan)
	handler.SetDisableGlobalCleanup(true)
	handler.SetLogger(logger)

	if err := handler.SetExternalServerMode(true); err != nil {
		t.Fatalf("failed to set external server mode: %v", err)
	}

	// Start the server so there is a running process to stop on restart
	go handler.StartServer(nil)

	// Give it time to compile and start
	time.Sleep(500 * time.Millisecond)

	// Now write a broken version (compile error)
	broken := fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
    "log"
)

func main() {
    fmt.rintf("BROKEN")
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "BROKEN")
    })
    log.Fatal(http.ListenAndServe(":%d", nil))
}`, port)

	if err := os.WriteFile(serverFile, []byte(broken), 0644); err != nil {
		t.Fatalf("writing broken server file: %v", err)
	}

	// Trigger file event - this should attempt restart and fail due to compile error
	err = handler.NewFileEvent("main.go", "go", serverFile, "write")
	if err == nil {
		t.Fatalf("expected error when restarting with broken code, got nil")
	}

	// Allow logs to be written
	time.Sleep(200 * time.Millisecond)

	// Now fix the file
	fixed := fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
    "log"
)

func main() {
    fmt.Println("VERSION_FIXED")
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "VERSION_FIXED")
    })
    log.Fatal(http.ListenAndServe(":%d", nil))
}`, port)

	if err := os.WriteFile(serverFile, []byte(fixed), 0644); err != nil {
		t.Fatalf("writing fixed server file: %v", err)
	}

	// Trigger file event again - this should succeed
	err = handler.NewFileEvent("main.go", "go", serverFile, "write")
	if err != nil {
		t.Fatalf("expected restart to succeed after fix, got error: %v", err)
	}

	// Give it time to recompile and start
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	logOutput := strings.Join(logMessages, "\n")
	mu.Unlock()
	if t.Failed() {
		t.Logf("Restart on fix logs: %s", logOutput)
	}

	// Stop server
	exitChan <- true
	time.Sleep(100 * time.Millisecond)
}

// Reuse the containsAny helper defined in compilation_test.go
