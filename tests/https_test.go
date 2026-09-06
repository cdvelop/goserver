package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webtyp.com/server"
)

func TestWaitForPortListening_HTTPS(t *testing.T) {
	// Start a mock HTTPS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// The httptest.Server URL will be something like https://127.0.0.1:PORT
	// Extract the port
	parts := strings.Split(ts.URL, ":")
	port := parts[len(parts)-1]

	// Test if WaitForPortListening can connect to it using HTTPS
	success := server.WaitForPortListening(port, 2*time.Second, true)
	if !success {
		t.Errorf("WaitForPortListening failed to connect to mock HTTPS server on port %s (URL: %s)", port, ts.URL)
	}
}

func TestWaitForPortListening_HTTP(t *testing.T) {
	// Start a mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Extract the port
	parts := strings.Split(ts.URL, ":")
	port := parts[len(parts)-1]

	// Test if WaitForPortListening can connect to it using HTTP
	success := server.WaitForPortListening(port, 2*time.Second, false)
	if !success {
		t.Errorf("WaitForPortListening failed to connect to mock HTTP server on port %s (URL: %s)", port, ts.URL)
	}
}
