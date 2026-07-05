package httpd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/router"
)

func TestSecurityClosedByDefault(t *testing.T) {
	srv := New(Config{})
	srv.Router().Get("/private", func(ctx router.Context) {
		ctx.Write([]byte("ok"))
	}) // No Public(), no Requires()

	handler, _ := srv.Handler()

	req := httptest.NewRequest("GET", "/private", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 for private route, got %d", w.Code)
	}
}

func TestSecurityPublicExplicit(t *testing.T) {
	srv := New(Config{})
	srv.Router().Get("/public", func(ctx router.Context) {
		ctx.Write([]byte("ok"))
	}).Public()

	handler, _ := srv.Handler()

	req := httptest.NewRequest("GET", "/public", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for public route, got %d", w.Code)
	}
}

func TestGlobalIdentityInModule(t *testing.T) {
	srv := New(Config{
		Authn: func(next router.HandlerFunc) router.HandlerFunc {
			return func(ctx router.Context) {
				ctx.SetUserID("u1")
				next(ctx)
			}
		},
	})

	var capturedID string
	srv.Router().Get("/mcp", func(ctx router.Context) {
		capturedID = ctx.UserID()
		ctx.Write([]byte("ok"))
	}) // No .Requires, but should have identity

	handler, _ := srv.Handler()

	req := httptest.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedID != "u1" {
		t.Errorf("Expected UserID 'u1', got %q", capturedID)
	}
}

func TestFailFastOnMissingAuthorize(t *testing.T) {
	srv := New(Config{})
	srv.Router().Get("/requires", func(ctx router.Context) {
	}).Requires("res", "act")

	_, err := srv.Handler()
	if err == nil {
		t.Error("Expected error when Route.Requires is used but Config.Authorize is missing")
	}
}
