package httpd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"webtyp.com/model"
	"webtyp.com/router"
)

func TestSecurityClosedByDefault(t *testing.T) {
	srv := New(Config{
		Authorize: func(u string, r model.Resource, a model.Action) bool { return true },
	})
	srv.Router().Get("/private", func(ctx router.Context) {
		ctx.Write([]byte("ok"))
	}).Requires(model.Resource("res"), model.Read) // Explicitly guarded

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
	}).Authenticated() // No resource, just identity

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
	}).Requires(model.Resource("res"), model.Read)

	_, err := srv.Handler()
	if err == nil {
		t.Error("Expected error when Route.Requires is used but Config.Authorize is missing")
	}
}
