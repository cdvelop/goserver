package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockStore struct {
	data map[string]string
}

func (m *mockStore) Get(key string) (string, error) { return m.data[key], nil }
func (m *mockStore) Set(key, value string) error    { m.data[key] = value; return nil }

func TestServerModeHandler_Label(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"
	h := New(cfg)
	db := &mockStore{data: make(map[string]string)}
	smh := NewServerModeHandler(h, db, nil)

	// Case 1: Internal Mode
	if label := smh.Label(); label != "SERVER → Switch to External" {
		t.Errorf("expected internal label, got: %s", label)
	}

	// Case 2: External Mode (unmodified)
	db.Set(StoreKeyExternalServer, "true")
	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("failed to set external mode: %v", err)
	}
	if label := smh.Label(); label != "SERVER → Switch to Internal" {
		t.Errorf("expected external unmodified label, got: %s", label)
	}

	// Case 3: External Mode (modified)
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	if err := os.WriteFile(targetPath, []byte("package main\n\nfunc main() { /* customized */ }\n"), 0644); err != nil {
		t.Fatalf("failed to modify server file: %v", err)
	}
	if label := smh.Label(); label != "SERVER: External (customized)" {
		t.Errorf("expected external customized label, got: %s", label)
	}
}

func TestSetExternalServerMode_SwitchesToExternal(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"
	cfg.OutputDir = "bin"

	h := New(cfg)

	if !h.executionInternal {
		t.Fatal("expected initial executionInternal to be true")
	}

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	if h.executionInternal {
		t.Fatal("expected executionInternal to be false after switching to external")
	}

	// Verify file was generated
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatalf("expected server file to be generated at %s", targetPath)
	}
}

func TestSetExternalServerMode_SwitchesToInternal_Unmodified(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	if err := h.SetExternalServerMode(false); err != nil {
		t.Fatalf("expected no error switching back to internal when unmodified, got: %v", err)
	}

	if !h.executionInternal {
		t.Fatal("expected executionInternal to be true after switching back to internal")
	}
}

func TestSetExternalServerMode_BlocksInternal_WhenModified(t *testing.T) {
	tmpData := t.TempDir()
	cfg := NewConfig()
	cfg.AppRootDir = tmpData
	cfg.SourceDir = "src"

	h := New(cfg)

	if err := h.SetExternalServerMode(true); err != nil {
		t.Fatalf("unexpected error switching to external: %v", err)
	}

	// Modify generated file
	targetPath := filepath.Join(tmpData, "src", cfg.MainInputFile)
	err := os.WriteFile(targetPath, []byte("package main\n\nfunc main() { /* modified */ }\n"), 0644)
	if err != nil {
		t.Fatalf("failed to modify server file: %v", err)
	}

	err = h.SetExternalServerMode(false)
	if err == nil {
		t.Fatal("expected error when switching to internal after modification, but got nil")
	}

	if !strings.Contains(err.Error(), "customized") {
		t.Errorf("expected error message to mention 'customized', got: %v", err)
	}

	if h.executionInternal {
		t.Fatal("expected executionInternal to remain false after failed switch")
	}
}

func TestSetCompilationOnDisk(t *testing.T) {
	h := New(nil)

	if h.compilationOnDisk {
		t.Fatal("expected default compilationOnDisk to be false")
	}

	h.SetCompilationOnDisk(true)
	if !h.compilationOnDisk {
		t.Fatal("expected compilationOnDisk to be true after setting")
	}
}
