package server_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"webtyp.com/server"
)

type recordingStore struct {
	writes []string
}

func (s *recordingStore) Get(string) (string, error) { return "", nil }
func (s *recordingStore) Set(key, _ string) error    { s.writes = append(s.writes, key); return nil }

// TestModoExternoLoDecideElArchivo: con <AppRootDir>/<SourceDir>/<MainInputFile>
// presente, StartServer debe elegir la estrategia externa.
func TestModoExternoLoDecideElArchivo(t *testing.T) {
	tmpData := t.TempDir()
	svrDir := filepath.Join(tmpData, "web")
	if err := os.MkdirAll(svrDir, 0755); err != nil {
		t.Fatal(err)
	}

	serverFilePath := filepath.Join(svrDir, "server.go")
	if err := os.WriteFile(serverFilePath, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	exitChan := make(chan bool, 1)
	exitChan <- true // Prevent blocking on StartServer

	h := server.New()
	h.SetAppRootDir(tmpData)
	h.SetSourceDir("web")
	h.SetMainInputFile("server.go")
	h.SetExitChan(exitChan)

	var wg sync.WaitGroup
	wg.Add(1)
	h.StartServer(&wg)

	if h.Value() != "external" {
		t.Fatalf("StartServer with existing %s: got Value()=%q, want external", serverFilePath, h.Value())
	}
}

// TestModoInternoSinArchivo: sin el archivo, StartServer se queda en la
// estrategia interna.
func TestModoInternoSinArchivo(t *testing.T) {
	tmpData := t.TempDir()

	exitChan := make(chan bool, 1)
	exitChan <- true

	h := server.New()
	h.SetAppRootDir(tmpData)
	h.SetSourceDir("web")
	h.SetMainInputFile("server.go")
	h.SetPort("19092")
	h.SetExitChan(exitChan)

	var wg sync.WaitGroup
	wg.Add(1)
	h.StartServer(&wg)

	if h.Value() != "internal" {
		t.Fatalf("StartServer without server file: got Value()=%q, want internal", h.Value())
	}
}

// TestNoSeEscribeNingunaClaveDeModo: un Store espía no recibe ninguna
// escritura durante StartServer, exista o no el archivo de servidor.
func TestNoSeEscribeNingunaClaveDeModo(t *testing.T) {
	tests := []struct {
		name     string
		withFile bool
	}{
		{"con archivo", true},
		{"sin archivo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpData := t.TempDir()
			svrDir := filepath.Join(tmpData, "web")
			if err := os.MkdirAll(svrDir, 0755); err != nil {
				t.Fatal(err)
			}

			serverFilePath := filepath.Join(svrDir, "server.go")
			if tt.withFile {
				if err := os.WriteFile(serverFilePath, []byte("package main"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			exitChan := make(chan bool, 1)
			exitChan <- true

			store := &recordingStore{}
			h := server.New()
			h.SetAppRootDir(tmpData)
			h.SetSourceDir("web")
			h.SetMainInputFile("server.go")
			h.SetPort("19093")
			h.SetExitChan(exitChan)
			h.SetStore(store)

			var wg sync.WaitGroup
			wg.Add(1)
			h.StartServer(&wg)

			if len(store.writes) != 0 {
				t.Fatalf("Store received %d writes during StartServer: %v (want none)", len(store.writes), store.writes)
			}
		})
	}
}

// TestLaDecisionSeRegistraConLaRutaCompleta: el log de arranque debe contener
// la ruta absoluta comprobada, en ambas direcciones (externa e interna).
func TestLaDecisionSeRegistraConLaRutaCompleta(t *testing.T) {
	tests := []struct {
		name       string
		withFile   bool
		wantSubstr string
	}{
		{"existe", true, "External mode: found"},
		{"no existe", false, "Internal mode:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpData := t.TempDir()
			svrDir := filepath.Join(tmpData, "web")
			if err := os.MkdirAll(svrDir, 0755); err != nil {
				t.Fatal(err)
			}

			serverFilePath := filepath.Join(svrDir, "server.go")
			if tt.withFile {
				if err := os.WriteFile(serverFilePath, []byte("package main"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			exitChan := make(chan bool, 1)
			exitChan <- true

			var logs []string
			var logsMu sync.Mutex
			logger := func(msgs ...any) {
				logsMu.Lock()
				defer logsMu.Unlock()
				for _, m := range msgs {
					logs = append(logs, fmt.Sprintf("%v", m))
				}
			}

			h := server.New()
			h.SetAppRootDir(tmpData)
			h.SetSourceDir("web")
			h.SetMainInputFile("server.go")
			h.SetPort("19094")
			h.SetExitChan(exitChan)
			h.SetLogger(logger)

			var wg sync.WaitGroup
			wg.Add(1)
			h.StartServer(&wg)

			logsMu.Lock()
			defer logsMu.Unlock()
			allLogs := strings.Join(logs, "\n")

			if !strings.Contains(allLogs, tt.wantSubstr) {
				t.Fatalf("launch log missing %q; got:\n%s", tt.wantSubstr, allLogs)
			}
			if !strings.Contains(allLogs, serverFilePath) {
				t.Fatalf("launch log missing absolute path %q; got:\n%s", serverFilePath, allLogs)
			}
		})
	}
}
