package server_test

import (
	"os"
	"testing"

	"github.com/tinywasm/server"
)

func TestMain(m *testing.M) {
	// Enable TestMode globally for all tests in this package.
	// This reduces timeouts (shutdown, port free check, etc.) to 100ms
	// preventing races and speeding up the entire suite.
	server.TestMode = true

	code := m.Run()
	os.Exit(code)
}
