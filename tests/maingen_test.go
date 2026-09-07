package server_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"webtyp.com/server"
)

func writeModule(t *testing.T, root, modulePath string) {
	t.Helper()
	src := "module " + modulePath + "\n\ngo 1.25\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(src), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
}

func writeRoutes(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "routes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "routes.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing routes.go: %v", err)
	}
}

func generatedImports(t *testing.T, file string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading generated main: %v", err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), file, src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("generated main does not parse: %v", err)
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		out = append(out, imp.Path.Value)
	}
	return out
}

// Test 1 — GenerateMain writes a file that parses and imports <modulePath>/routes.
func TestGenerateMain_ParsesAndImportsRoutes(t *testing.T) {
	root := t.TempDir()
	const modulePath = "example.com/app"
	writeModule(t, root, modulePath)
	writeRoutes(t, root, "package routes\n\nimport \"webtyp.com/router\"\n\nfunc Register(r router.Router) {}\n")

	got, err := server.GenerateMain(root, modulePath, server.MainConfig{
		Port:      "8080",
		PublicDir: "web/public",
		DevTLS:    true,
	})
	if err != nil {
		t.Fatalf("GenerateMain: %v", err)
	}

	want := filepath.Join(root, server.GeneratedMainDir, "main.go")
	if got != want {
		t.Errorf("GenerateMain returned %q, want %q", got, want)
	}

	imports := generatedImports(t, got)
	wantImport := `"` + modulePath + `/routes"`
	found := false
	for _, imp := range imports {
		if imp == wantImport {
			found = true
		}
	}
	if !found {
		t.Errorf("generated main does not import %s; imports: %v", wantImport, imports)
	}

	src, _ := os.ReadFile(got)
	if !strings.Contains(string(src), "DO NOT EDIT") {
		t.Errorf("generated main missing the generated-code marker")
	}
}

// Test 5 — GenerateMain is idempotent: a second run produces the same bytes and
// no error.
func TestGenerateMain_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "example.com/app")
	writeRoutes(t, root, "package routes\n\nimport \"webtyp.com/router\"\n\nfunc Register(r router.Router) {}\n")

	cfg := server.MainConfig{Port: "8080", PublicDir: "web/public"}
	first, err := server.GenerateMain(root, "example.com/app", cfg)
	if err != nil {
		t.Fatalf("GenerateMain #1: %v", err)
	}
	a, _ := os.ReadFile(first)

	if _, err := server.GenerateMain(root, "example.com/app", cfg); err != nil {
		t.Fatalf("GenerateMain #2: %v", err)
	}
	b, _ := os.ReadFile(first)

	if string(a) != string(b) {
		t.Errorf("GenerateMain not idempotent:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}
}

// Test 2 — HasRoutes is false without routes/routes.go and true with it.
func TestHasRoutes(t *testing.T) {
	root := t.TempDir()
	if server.HasRoutes(root) {
		t.Fatal("HasRoutes = true for an empty project")
	}
	writeRoutes(t, root, "package routes\n")
	if !server.HasRoutes(root) {
		t.Fatal("HasRoutes = false after creating routes/routes.go")
	}
}

// Test 4 — when web/server.go exists it is the compile input and GenerateMain is
// not called (the file is never overwritten).
func TestEscapeHatch_UserMainWins(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "example.com/app")
	writeRoutes(t, root, "package routes\n\nimport \"webtyp.com/router\"\n\nfunc Register(r router.Router) {}\n")

	webDir := filepath.Join(root, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const userMain = "package main // hand written\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(webDir, "server.go"), []byte(userMain), 0o644); err != nil {
		t.Fatal(err)
	}

	exit := make(chan bool, 1)
	exit <- true
	h := server.New()
	h.SetAppRootDir(root)
	h.SetSourceDir("web")
	h.SetMainInputFile("server.go")
	h.SetHTTPS(false)
	h.SetPort("0")
	h.SetExitChan(exit)
	h.SetLogger(t.Log)

	var wg sync.WaitGroup
	wg.Add(1)
	h.StartServer(&wg)
	wg.Wait()

	// The user's file is untouched...
	got, _ := os.ReadFile(filepath.Join(webDir, "server.go"))
	if string(got) != userMain {
		t.Errorf("user server.go was modified:\n%s", got)
	}
	// ...and nothing was generated.
	if _, err := os.Stat(filepath.Join(root, server.GeneratedMainDir, "main.go")); err == nil {
		t.Error("GenerateMain ran even though web/server.go exists")
	}
}
