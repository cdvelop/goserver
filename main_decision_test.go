package server

import (
	"os"
	"path/filepath"
	"testing"
)

func mkModule(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkRoutes(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "routes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "routes.go"),
		[]byte("package routes\nimport \"webtyp.com/router\"\nfunc Register(r router.Router) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkUserMain(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "web")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newDecisionHandler(root string) *ServerHandler {
	h := New()
	h.SetAppRootDir(root)
	h.SetSourceDir("web")
	h.SetMainInputFile("server.go")
	h.SetHTTPS(false)
	return h
}

// Test 2/4 — strategy selection is decided by routes/routes.go, with an existing
// web/server.go as the escape hatch that wins and is never generated over.
func TestServerMainSelection(t *testing.T) {
	t.Run("no routes, no user main -> internal, nothing generated", func(t *testing.T) {
		root := t.TempDir()
		mkModule(t, root)
		h := newDecisionHandler(root)

		if h.needsExternalProcess() {
			t.Error("needsExternalProcess = true with no routes and no user main")
		}
		if err := h.ensureServerMain(false); err != nil {
			t.Fatalf("ensureServerMain: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, GeneratedMainDir, GeneratedMainFilename)); err == nil {
			t.Error("a main was generated for a project without routes")
		}
	})

	t.Run("routes present -> external via generated main", func(t *testing.T) {
		root := t.TempDir()
		mkModule(t, root)
		mkRoutes(t, root)
		h := newDecisionHandler(root)

		if !h.usesGeneratedMain() {
			t.Fatal("usesGeneratedMain = false with routes/routes.go present")
		}
		if !h.needsExternalProcess() {
			t.Fatal("needsExternalProcess = false with routes/routes.go present")
		}
		if got, want := h.serverMainRelPath(), filepath.Join(GeneratedMainDir, GeneratedMainFilename); got != want {
			t.Errorf("serverMainRelPath = %q, want %q", got, want)
		}
		if err := h.ensureServerMain(false); err != nil {
			t.Fatalf("ensureServerMain: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, GeneratedMainDir, GeneratedMainFilename)); err != nil {
			t.Errorf("generated main missing: %v", err)
		}
	})

	t.Run("web/server.go is the escape hatch and wins", func(t *testing.T) {
		root := t.TempDir()
		mkModule(t, root)
		mkRoutes(t, root) // even with routes present...
		mkUserMain(t, root)
		h := newDecisionHandler(root)

		if !h.hasHandWrittenMain() {
			t.Fatal("hasHandWrittenMain = false with web/server.go present")
		}
		if h.usesGeneratedMain() {
			t.Error("usesGeneratedMain = true although web/server.go exists")
		}
		if got, want := h.serverMainRelPath(), filepath.Join("web", "server.go"); got != want {
			t.Errorf("serverMainRelPath = %q, want %q", got, want)
		}

		const before = "package main\nfunc main() {}\n"
		for _, force := range []bool{false, true} {
			if err := h.ensureServerMain(force); err != nil {
				t.Fatalf("ensureServerMain(%v): %v", force, err)
			}
		}
		got, _ := os.ReadFile(filepath.Join(root, "web", "server.go"))
		if string(got) != before {
			t.Errorf("web/server.go was modified:\n%s", got)
		}
		if _, err := os.Stat(filepath.Join(root, GeneratedMainDir, GeneratedMainFilename)); err == nil {
			t.Error("a main was generated even though web/server.go exists")
		}
	})
}

// Test 5 — ensureServerMain records .build/ through the SetGitIgnoreAdd hook.
func TestEnsureServerMain_GitIgnoreHook(t *testing.T) {
	root := t.TempDir()
	mkModule(t, root)
	mkRoutes(t, root)

	var got []string
	h := newDecisionHandler(root)
	h.SetGitIgnoreAdd(func(entry string) error {
		got = append(got, entry)
		return nil
	})

	for i := 0; i < 2; i++ {
		if err := h.ensureServerMain(false); err != nil {
			t.Fatalf("ensureServerMain #%d: %v", i, err)
		}
	}

	seen := false
	for _, e := range got {
		if e == BuildDirGitIgnore {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("SetGitIgnoreAdd never received %q; got %v", BuildDirGitIgnore, got)
	}
}
