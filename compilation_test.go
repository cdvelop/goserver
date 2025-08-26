package goserver

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestStartServerAlwaysRecompiles verifica que StartServer siempre recompile el servidor
// incluso si el ejecutable ya existe
func TestStartServerAlwaysRecompiles(t *testing.T) {
	tmp := t.TempDir()

	var logBuf safeBuffer

	// Create public folder
	publicDir := filepath.Join(tmp, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("creating public folder: %v", err)
	}

	cfg := &Config{
		RootFolder:                  tmp,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     "0", // Use port 0 for automatic assignment
		Logger:                      &logBuf,
		ExitChan:                    make(chan bool, 1),
	}

	handler := New(cfg)

	// First, create the server file
	serverFile := filepath.Join(tmp, "main.server.go")
	initialContent := `package main

import (
	"fmt"
	"net/http"
	"log"
)

func main() {
	fmt.Println("VERSION_1")
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "VERSION_1")
	})
	log.Fatal(http.ListenAndServe(":0", nil))
}`

	if err := os.WriteFile(serverFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("writing initial server file: %v", err)
	}

	// Start the server for the first time
	var wg sync.WaitGroup
	wg.Add(1)
	go handler.StartServer(&wg)
	wg.Wait()

	// Give it time to compile and start
	time.Sleep(500 * time.Millisecond)

	// Stop the server
	cfg.ExitChan <- true
	time.Sleep(100 * time.Millisecond)

	// Now modify the server file
	modifiedContent := `package main

import (
	"fmt"
	"net/http"
	"log"
)

func main() {
	fmt.Println("VERSION_2")
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "VERSION_2")
	})
	log.Fatal(http.ListenAndServe(":0", nil))
}`

	if err := os.WriteFile(serverFile, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("writing modified server file: %v", err)
	}

	// Clear the log buffer to check for new compilation
	logBuf.Reset()

	// Start the server again - it should recompile
	cfg.ExitChan = make(chan bool, 1) // Reset exit channel
	wg.Add(1)
	go handler.StartServer(&wg)
	wg.Wait()

	// Give it time to compile and start
	time.Sleep(500 * time.Millisecond)

	// Check logs to ensure compilation happened
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("Expected compilation logs but got none")
	}

	// Stop the server
	cfg.ExitChan <- true
	time.Sleep(100 * time.Millisecond)

	t.Logf("Compilation logs: %s", logOutput)
}

// TestNewFileEventTriggersRecompilation verifica que NewFileEvent recompile cuando se notifica
func TestNewFileEventTriggersRecompilation(t *testing.T) {
	tmp := t.TempDir()

	var logBuf safeBuffer

	// Create public folder
	publicDir := filepath.Join(tmp, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("creating public folder: %v", err)
	}

	cfg := &Config{
		RootFolder:                  tmp,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     "0", // Use port 0 for automatic assignment
		Logger:                      &logBuf,
		ExitChan:                    make(chan bool, 1),
	}

	handler := New(cfg)

	// Create the server file
	serverFile := filepath.Join(tmp, "main.server.go")
	initialContent := `package main

import (
	"fmt"
	"net/http"
	"log"
)

func main() {
	fmt.Println("INITIAL_VERSION")
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "INITIAL_VERSION")
	})
	log.Fatal(http.ListenAndServe(":0", nil))
}`

	if err := os.WriteFile(serverFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("writing initial server file: %v", err)
	}

	// Start the server first
	var wg sync.WaitGroup
	wg.Add(1)
	go handler.StartServer(&wg)
	wg.Wait()

	// Give it time to compile and start
	time.Sleep(500 * time.Millisecond)

	// Clear the log buffer to check for recompilation triggered by file event
	logBuf.Reset()

	// Simulate a file write event on the main server file
	err := handler.NewFileEvent("main.server.go", "go", serverFile, "write")
	if err != nil {
		t.Fatalf("NewFileEvent failed: %v", err)
	}

	// Give it time to recompile and restart
	time.Sleep(500 * time.Millisecond)

	// Check logs to ensure recompilation happened
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("Expected recompilation logs but got none")
	}

	// The log should contain messages about restarting
	if !containsAny(logOutput, []string{"External server modified", "restarting"}) {
		t.Errorf("Expected restart messages in logs, got: %s", logOutput)
	}

	// Stop the server
	cfg.ExitChan <- true
	time.Sleep(100 * time.Millisecond)

	t.Logf("File event logs: %s", logOutput)
}

// TestNewFileEventOnOtherGoFiles verifica que cambios en otros archivos Go también recompilen
func TestNewFileEventOnOtherGoFiles(t *testing.T) {
	tmp := t.TempDir()

	var logBuf safeBuffer

	// Create public folder
	publicDir := filepath.Join(tmp, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("creating public folder: %v", err)
	}

	cfg := &Config{
		RootFolder:                  tmp,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     "0",
		Logger:                      &logBuf,
		ExitChan:                    make(chan bool, 1),
	}

	handler := New(cfg)

	// Create the server file
	serverFile := filepath.Join(tmp, "main.server.go")
	serverContent := `package main

import (
	"fmt"
	"net/http"
	"log"
)

func main() {
	fmt.Println("SERVER_VERSION")
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "SERVER_VERSION")
	})
	log.Fatal(http.ListenAndServe(":0", nil))
}`

	if err := os.WriteFile(serverFile, []byte(serverContent), 0644); err != nil {
		t.Fatalf("writing server file: %v", err)
	}

	// Create another Go file (shared module)
	sharedFile := filepath.Join(tmp, "utils.go")
	sharedContent := `package main

func UtilFunction() string {
	return "utility"
}`

	if err := os.WriteFile(sharedFile, []byte(sharedContent), 0644); err != nil {
		t.Fatalf("writing shared file: %v", err)
	}

	// Start the server first
	var wg sync.WaitGroup
	wg.Add(1)
	go handler.StartServer(&wg)
	wg.Wait()

	// Give it time to compile and start
	time.Sleep(500 * time.Millisecond)

	// Clear the log buffer
	logBuf.Reset()

	// Simulate a file write event on the shared Go file
	err := handler.NewFileEvent("utils.go", "go", sharedFile, "write")
	if err != nil {
		t.Fatalf("NewFileEvent failed: %v", err)
	}

	// Give it time to recompile and restart
	time.Sleep(500 * time.Millisecond)

	// Check logs to ensure recompilation happened
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("Expected recompilation logs but got none")
	}

	// The log should contain messages about restarting
	if !containsAny(logOutput, []string{"Go file modified", "restarting"}) {
		t.Errorf("Expected restart messages in logs, got: %s", logOutput)
	}

	// Stop the server
	cfg.ExitChan <- true
	time.Sleep(100 * time.Millisecond)

	t.Logf("Shared file event logs: %s", logOutput)
}

// Helper function to check if a string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) > 0 && len(substr) > 0 {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
