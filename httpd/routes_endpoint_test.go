package httpd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutesEndpointDisabled(t *testing.T) {
	cfg := Config{
		RoutesEndpoint: false,
	}
	s := New(cfg)

	// In a real s.ListenAndServe(), this is called.
	s.registerRoutesEndpoint()

	// Use the server's mux directly to verify the endpoint is NOT registered.
	req := httptest.NewRequest("GET", RoutesPath, nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found when RoutesEndpoint is disabled, got %d", w.Code)
	}
}
