package httpd

import (
	"strings"
	"testing"

	"github.com/tinywasm/router"
)

func TestRBACMissingAuthorizerFailsAtStartup(t *testing.T) {
	cfg := Config{}
	s := New(cfg)

	// Register a route requiring a resource
	s.Router().Get("/secret", func(ctx router.Context) {}).Requires("secrets", "read")

	// Call validateRBAC, which is what ListenAndServe calls first
	err := s.validateRBAC()

	if err == nil {
		t.Fatal("Expected error because Authorizer is missing, but got nil")
	}

	if !strings.Contains(err.Error(), "secrets") {
		t.Errorf("Expected error message to contain resource 'secrets', got %q", err.Error())
	}
}
