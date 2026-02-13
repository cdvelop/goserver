package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/server"
)

func newTestHandler(t *testing.T, sourceDir, outputDir, appRootDir string) *server.ServerHandler {
	t.Helper()
	h := server.New().
		SetAppRootDir(appRootDir).
		SetSourceDir(sourceDir).
		SetOutputDir(outputDir).
		SetPort("9090").
		SetExitChan(make(chan bool, 10)).
		SetLogger(t.Log)
	return h
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
	target := filepath.Join(fullSourcePath, h.MainInputFile)
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("expected no existing file at %s", target)
	}

	// Signal exit immediately so CreateTemplateServer doesn't block on Start.
	// We close the channel so all workers (strategy, gorun) receive the signal.
	close(h.ExitChan)

	// Use CreateTemplateServer instead of internal generateServerFromEmbeddedMarkdown
	// It will stop internal, generate, compile, and run. We expect it to generate.
	// Compilation might fail in test env, but we only care about generation here.
	if err := h.CreateTemplateServer(); err != nil {
		// If error is just compilation failure, we can ignore it if file was generated
		t.Logf("CreateTemplateServer returned error: %v", err)
	}

	// Wait a bit for file to be generated?
	// CreateTemplateServer calls generate synchronously before Start.
	// So file should be there.

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
	if !strings.Contains(content, `*publicDir = "web/public"`) {
		t.Errorf("generated file missing substituted PublicDir (web/public)")
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
	target := filepath.Join(fullSourcePath, h.MainInputFile)

	orig := "__ORIGINAL__"
	if err := os.WriteFile(target, []byte(orig), 0644); err != nil {
		t.Fatalf("writing original file: %v", err)
	}

	// Signal exit immediately
	close(h.ExitChan)

	if err := h.CreateTemplateServer(); err != nil {
		t.Logf("CreateTemplateServer returned error: %v", err)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading file after generate: %v", err)
	}
	if string(b) != orig {
		t.Fatalf("file was overwritten, expected original content")
	}
}
