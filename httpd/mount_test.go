package httpd

import (
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/router"
)

type fakeAPIModule struct{}

func (m fakeAPIModule) ModelName() string {
	return "fake"
}

func (m fakeAPIModule) MountAPI(r router.Router) {
	r.Get("/mounted", func(ctx router.Context) {
		ctx.Write([]byte("mounted ok"))
	}).Public()
}

func TestMount(t *testing.T) {
	cfg := Config{}
	s := New(cfg)

	s.Mount(fakeAPIModule{})

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/mounted", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	if w.Body.String() != "mounted ok" {
		t.Errorf("Expected 'mounted ok', got %q", w.Body.String())
	}
}
