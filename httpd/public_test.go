package httpd

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/router"
)

func TestPublicAssetsAndDir(t *testing.T) {
	tmpDir := t.TempDir()
	publicDir := filepath.Join(tmpDir, "public")
	os.Mkdir(publicDir, 0755)
	os.WriteFile(filepath.Join(publicDir, "test.txt"), []byte("public content"), 0644)
	os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("index content"), 0644)

	s := New(Config{
		PublicDir: publicDir,
	})

	r := s.Router()
	r.PublicAsset("/asset.js", func(ctx router.Context) {
		ctx.Write([]byte("console.log('hi')"))
	})
	r.Get("/hello", func(ctx router.Context) {
		ctx.Write([]byte("world"))
	}).Public()
	r.Get("/private", func(ctx router.Context) {
		ctx.Write([]byte("secret"))
	})

	handler, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Check Routes()
	routes := r.Routes()
	foundDir := false
	foundAsset := false
	for _, info := range routes {
		if info.Path == "/" && info.Dir == publicDir && info.Public {
			foundDir = true
		}
		if info.Path == "/asset.js" && info.Public {
			foundAsset = true
		}
	}
	if !foundDir {
		t.Errorf("PublicDir route not found in Routes()")
	}
	if !foundAsset {
		t.Errorf("PublicAsset route not found in Routes()")
	}

	// 2. Specific route not shadowed
	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "world" {
		t.Errorf("GET /hello failed: got %d %q", w.Code, w.Body.String())
	}

	// 3. Static file served to anonymous
	req = httptest.NewRequest("GET", "/test.txt", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "public content" {
		t.Errorf("GET /test.txt failed: got %d %q", w.Code, w.Body.String())
	}

	// 4. Index.html fallback
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "index content" {
		t.Errorf("GET / failed: got %d %q", w.Code, w.Body.String())
	}

	// 5. Private route still 403
	req = httptest.NewRequest("GET", "/private", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("GET /private expected 403, got %d", w.Code)
	}
}

func TestNoPanicOnCollision(t *testing.T) {
	s := New(Config{})
	r := s.Router()

	// Should NOT panic
	r.PublicAsset("/", func(ctx router.Context) {
		ctx.Write([]byte("asset at root"))
	})
	r.PublicDir("/", "some/dir")

	handler, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}

	// Asset should win over Dir fallback
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "asset at root" {
		t.Errorf("Collision test: expected asset content, got %d %q", w.Code, w.Body.String())
	}
}
