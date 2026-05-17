package server_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/tinywasm/server"
)

// Mock strategy to verify timing and calls
type mockStrategy struct {
	startCalled bool
	startTime   time.Time
	mu          sync.Mutex
}

func (m *mockStrategy) Start(wg *sync.WaitGroup) error {
	m.mu.Lock()
	m.startCalled = true
	m.startTime = time.Now()
	m.mu.Unlock()
	if wg != nil {
		wg.Done()
	}
	return nil
}

func (m *mockStrategy) Stop() error { return nil }
func (m *mockStrategy) Restart() error { return nil }
func (m *mockStrategy) HandleFileEvent(fileName, extension, filePath, event string) error { return nil }
func (m *mockStrategy) Name() string { return "Mock" }

// Helper to set unexported fields via reflection
func setUnexportedField(h *server.ServerHandler, fieldName string, value interface{}) {
	v := reflect.ValueOf(h).Elem()
	f := v.FieldByName(fieldName)
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(value))
}

// External mode must invoke BeforeExternalServerStart BEFORE strategy.Start.
func TestStartServer_BeforeHookRunsBeforeStrategy(t *testing.T) {
	h := server.New()
	setUnexportedField(h, "executionInternal", false)
	mock := &mockStrategy{}
	setUnexportedField(h, "strategy", server.ServerStrategy(mock))

	var hookReturnTime time.Time
	h.SetBeforeExternalServerStart(func() error {
		time.Sleep(20 * time.Millisecond) // Ensure measurable difference
		hookReturnTime = time.Now()
		return nil
	})

	h.StartServer(nil)

	if !mock.startCalled {
		t.Fatal("strategy.Start was not called")
	}

	if hookReturnTime.IsZero() {
		t.Fatal("hook was not called")
	}

	if !hookReturnTime.Before(mock.startTime) && !hookReturnTime.Equal(mock.startTime) {
		t.Errorf("hook returned at %v, but strategy started at %v", hookReturnTime, mock.startTime)
	}
}

// A non-nil error from the hook must abort strategy.Start.
func TestStartServer_BeforeHookErrorAborts(t *testing.T) {
	var loggedError error
	h := server.New().SetLogger(func(args ...any) {
		for _, arg := range args {
			if err, ok := arg.(error); ok {
				loggedError = err
			}
		}
	})

	setUnexportedField(h, "executionInternal", false)
	mock := &mockStrategy{}
	setUnexportedField(h, "strategy", server.ServerStrategy(mock))

	hookErr := errors.New("boom")
	h.SetBeforeExternalServerStart(func() error {
		return hookErr
	})

	h.StartServer(nil)

	if mock.startCalled {
		t.Error("expected strategy.Start NOT to be called when hook returns error")
	}

	if loggedError != hookErr {
		t.Errorf("expected logged error %v, got %v", hookErr, loggedError)
	}
}

// Internal mode must NOT invoke BeforeExternalServerStart.
func TestStartServer_InternalModeSkipsBeforeHook(t *testing.T) {
	h := server.New()
	h.SetExitChan(make(chan bool, 1))
	h.ExitChan <- true // Allow Start to return immediately

	hookCalled := false
	h.SetBeforeExternalServerStart(func() error {
		hookCalled = true
		return nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	h.StartServer(&wg)

	if hookCalled {
		t.Error("expected hook NOT to be called in internal mode")
	}
}

// RestartServer must bypass the hook by design.
func TestRestartServer_DoesNotInvokeHook(t *testing.T) {
	h := server.New()
	setUnexportedField(h, "executionInternal", false)
	mock := &mockStrategy{}
	setUnexportedField(h, "strategy", server.ServerStrategy(mock))

	hookCount := 0
	h.SetBeforeExternalServerStart(func() error {
		hookCount++
		return nil
	})

	// 1. StartServer (hook fires)
	h.StartServer(nil)
	if hookCount != 1 {
		t.Errorf("expected hook count 1 after StartServer, got %d", hookCount)
	}

	// 2. RestartServer (hook should NOT fire)
	err := h.RestartServer()
	if err != nil {
		t.Fatalf("RestartServer failed: %v", err)
	}

	if hookCount != 1 {
		t.Errorf("expected hook count to remain 1 after RestartServer, got %d", hookCount)
	}
}

// Test idempotency: hook fires on every StartServer call in external mode.
func TestStartServer_HookIsIdempotent(t *testing.T) {
	h := server.New()
	setUnexportedField(h, "executionInternal", false)
	mock := &mockStrategy{}
	setUnexportedField(h, "strategy", server.ServerStrategy(mock))

	hookCount := 0
	h.SetBeforeExternalServerStart(func() error {
		hookCount++
		return nil
	})

	h.StartServer(nil)
	h.StartServer(nil)

	if hookCount != 2 {
		t.Errorf("expected hook to fire twice for two StartServer calls, got %d", hookCount)
	}
}
