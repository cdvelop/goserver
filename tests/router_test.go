package server_test

import (
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"strings"

	"webtyp.com/router"
	"webtyp.com/server"
)

func TestRouterRegistration(t *testing.T) {
	tmp := t.TempDir()
	h := server.New()
	h.SetAppRootDir(tmp)
	h.SetPort("9091")
	h.SetLogger(t.Log)

	h.RegisterRoutes(func(r router.Router) {
		r.Get("/hello", func(ctx router.Context) {
			ctx.Write([]byte("world"))
		}).Public()
		r.Post("/echo", func(ctx router.Context) {
			ctx.Write(ctx.Body())
		}).Public()
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go h.StartServer(&wg)

	// Wait for server to be ready
	if !server.WaitForPortListening("9091", 5*time.Second, false) {
		t.Fatal("Server failed to start")
	}

	// Test GET /hello
	resp, err := http.Get("http://localhost:9091/hello")
	if err != nil {
		t.Fatalf("GET /hello failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "world" {
		t.Errorf("expected 'world', got %q", string(body))
	}

	// Test POST /echo
	respPost, err := http.Post("http://localhost:9091/echo", "text/plain", strings.NewReader("hello echo"))
	if err != nil {
		t.Fatalf("POST /echo failed: %v", err)
	}
	defer respPost.Body.Close()
	bodyPost, _ := io.ReadAll(respPost.Body)
	if string(bodyPost) != "hello echo" {
		t.Errorf("expected 'hello echo', got %q", string(bodyPost))
	}

	h.StopServer()

	// Signal exit
	h.ExitChan <- true

	wg.Wait()
}

func TestRouterMiddleware(t *testing.T) {
	tmp := t.TempDir()
	h := server.New()
	h.SetAppRootDir(tmp)
	h.SetPort("9092")
	h.SetLogger(t.Log)

	h.RegisterRoutes(func(r router.Router) {
		r.Use(func(next router.HandlerFunc) router.HandlerFunc {
			return func(ctx router.Context) {
				ctx.SetHeader("X-Middleware", "true")
				next(ctx)
			}
		})
		r.Get("/mid", func(ctx router.Context) {
			ctx.Write([]byte("ok"))
		}).Public()
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go h.StartServer(&wg)

	if !server.WaitForPortListening("9092", 5*time.Second, false) {
		t.Fatal("Server failed to start")
	}

	resp, err := http.Get("http://localhost:9092/mid")
	if err != nil {
		t.Fatalf("GET /mid failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Middleware") != "true" {
		t.Errorf("expected X-Middleware: true header")
	}

	h.StopServer()

	// Signal exit
	h.ExitChan <- true

	wg.Wait()
}
