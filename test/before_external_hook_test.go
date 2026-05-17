package server_test

// Reproducer test suite for tinywasm/app docs/PLAN.md §1 (rename of
// OnExternalModeExecution → BeforeExternalServerStart, with synchronous
// error-returning contract).
//
// Skipped today because SetBeforeExternalServerStart does not yet exist.
// The external agent implementing the PLAN MUST remove t.Skip and adapt.

import (
	"testing"
)

// External mode must invoke BeforeExternalServerStart BEFORE strategy.Start.
func TestStartServer_BeforeHookRunsBeforeStrategy(t *testing.T) {
	t.Skip("see app/docs/PLAN.md — requires BeforeExternalServerStart hook")

	// TODO(agent):
	// 1. Build a ServerHandler with a fake strategy that records Start() time.
	// 2. Register a BeforeExternalServerStart hook that records its return time.
	// 3. Place web/server.go stub so external mode triggers.
	// 4. Call StartServer; assert hookReturnTime <= strategyStartTime.
}

// A non-nil error from the hook must abort strategy.Start.
func TestStartServer_BeforeHookErrorAborts(t *testing.T) {
	t.Skip("see app/docs/PLAN.md — requires error propagation contract")

	// TODO(agent):
	// Register a hook that returns errors.New("boom"); assert strategy.Start was
	// never invoked and that the error was logged.
}

// Internal mode must NOT invoke BeforeExternalServerStart.
func TestStartServer_InternalModeSkipsBeforeHook(t *testing.T) {
	t.Skip("see app/docs/PLAN.md — requires conditional hook invocation")

	// TODO(agent):
	// Configure for internal mode (no web/server.go on disk); register a hook
	// that flips a flag; assert the flag remains false after StartServer.
}

// RestartServer must bypass the hook by design (app/docs/PLAN.md §1).
// A restart in external mode does not need re-flushing because assetmin is
// already in disk-mirrored mode.
func TestRestartServer_DoesNotInvokeHook(t *testing.T) {
	t.Skip("see app/docs/PLAN.md — requires RestartServer to bypass the hook")

	// TODO(agent):
	// 1. Boot in external mode (hook fires once during StartServer).
	// 2. Call h.RestartServer().
	// 3. Assert the hook counter did not increment.
}
