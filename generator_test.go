package goserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T, sourceDir, outputDir, appRootDir string) *ServerHandler {
	t.Helper()
	cfg := &Config{
		AppRootDir: appRootDir,
		SourceDir:  sourceDir,
		OutputDir:  outputDir,
		AppPort:    "9090",
		Logger:     func(messages ...any) { fmt.Fprintln(os.Stdout, messages...) },
		ExitChan:   make(chan bool),
	}
	return New(cfg)
}

func TestGenerateCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	sourceDir := "src/app"
	outputDir := "deploy"
	fullSourcePath := filepath.Join(tmp, sourceDir)
	if err := os.MkdirAll(fullSourcePath, 0755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	h := newTestHandler(t, sourceDir, outputDir, tmp)

	// Ensure no existing file
	target := filepath.Join(fullSourcePath, h.mainFileExternalServer)
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("expected no existing file at %s", target)
	}

	if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "package main") {
		t.Errorf("generated file missing package main")
	}
	if !strings.Contains(content, "9090") {
		t.Errorf("generated file missing substituted AppPort (9090)")
	}
	// Verify it uses flag package
	if !strings.Contains(content, `flag.String`) {
		t.Errorf("generated file missing flag.String usage")
	}
	// Verify it has the new flag-based configuration
	if !strings.Contains(content, `publicDir := flag.String("public-dir"`) {
		t.Errorf("generated file missing public-dir flag")
	}
	if !strings.Contains(content, `port := flag.String("port"`) {
		t.Errorf("generated file missing port flag")
	}
	// Verify it uses filepath.Abs for path resolution
	if !strings.Contains(content, `filepath.Abs`) {
		t.Errorf("generated file missing filepath.Abs call")
	}
	// Verify noCache middleware exists
	if !strings.Contains(content, `noCache`) {
		t.Errorf("generated file missing noCache middleware")
	}
	// Verify default public dir
	if !strings.Contains(content, `*publicDir = "public"`) {
		t.Errorf("generated file missing default public dir assignment")
	}
}

func TestGenerateDoesNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	sourceDir := "src/app"
	outputDir := "deploy"
	fullSourcePath := filepath.Join(tmp, sourceDir)
	if err := os.MkdirAll(fullSourcePath, 0755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	h := newTestHandler(t, sourceDir, outputDir, tmp)
	target := filepath.Join(fullSourcePath, h.mainFileExternalServer)

	orig := "__ORIGINAL__"
	if err := os.WriteFile(target, []byte(orig), 0644); err != nil {
		t.Fatalf("writing original file: %v", err)
	}

	if err := h.generateServerFromEmbeddedMarkdown(); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file after generate: %v", err)
	}
	if string(b) != orig {
		t.Fatalf("file was overwritten, expected original content")
	}
}
