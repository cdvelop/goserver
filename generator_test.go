package goserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T, root string) *ServerHandler {
	t.Helper()
	cfg := &Config{
		RootFolder:                  root,
		MainFileWithoutExtension:    "main.server",
		ArgumentsForCompilingServer: nil,
		ArgumentsToRunServer:        nil,
		PublicFolder:                "public",
		AppPort:                     "9090",
		Logger:                      os.Stdout,
		ExitChan:                    make(chan bool),
	}
	return New(cfg)
}

func TestGenerateCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, tmp)

	// Ensure no existing file
	target := filepath.Join(tmp, h.mainFileExternalServer)
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
	if !strings.Contains(content, "public") {
		t.Errorf("generated file missing substituted PublicFolder (public)")
	}
}

func TestGenerateDoesNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	h := newTestHandler(t, tmp)
	target := filepath.Join(tmp, h.mainFileExternalServer)

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

func TestExtractConcatenation(t *testing.T) {
	// Use a dummy handler
	h := newTestHandler(t, t.TempDir())

	md := "Some text\n```go\npackage main\n\nfunc A(){}\n```\nMore\n```go\nfunc B(){}\n```\n"
	out := h.extractGoCodeFromMarkdown(md)
	if !strings.Contains(out, "func A()") || !strings.Contains(out, "func B()") {
		t.Fatalf("extraction failed, got: %s", out)
	}
	// Ensure both blocks concatenated
	if strings.Count(out, "func") < 2 {
		t.Fatalf("expected both functions present, got: %s", out)
	}
}
