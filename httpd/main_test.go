package httpd

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Never touch the developer's real OS trust store from a test run — that
	// path shells out to `sudo` on Linux and prompts for a password.
	os.Setenv(EnvSkipTruststore, "1")
	os.Exit(m.Run())
}
