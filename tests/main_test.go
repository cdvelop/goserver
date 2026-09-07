package server_test

import (
	"os"
	"testing"

	"webtyp.com/server"
	"webtyp.com/server/httpd"
)

func TestMain(m *testing.M) {
	// Enable TestMode globally for all tests in this package.
	// This reduces timeouts (shutdown, port free check, etc.) to 100ms
	// preventing races and speeding up the entire suite.
	server.TestMode = true

	// A test run must never install a CA into the developer's OS trust store —
	// that path prompts for a root password.
	os.Setenv(httpd.EnvSkipTruststore, "1")

	code := m.Run()
	os.Exit(code)
}
