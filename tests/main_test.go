package server_test

import (
	"os"
	"testing"

	"webtyp.com/server"
)

func TestMain(m *testing.M) {
	// Enable TestMode globally for all tests in this package.
	// This reduces timeouts (shutdown, port free check, etc.) to 100ms
	// preventing races and speeding up the entire suite.
	server.TestMode = true

	code := m.Run()
	os.Exit(code)
}
