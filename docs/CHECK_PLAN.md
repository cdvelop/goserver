# PLAN: Close the `CreateTemplateServer` hook gap

> Status: Ready for execution. Follow-up to the merged `BeforeExternalServerStart`
> refactor (see [CHECK_PLAN.md](CHECK_PLAN.md)). Breaking-free additive fix.

## Context

The previous PLAN (now [CHECK_PLAN.md](CHECK_PLAN.md)) replaced the asynchronous
`OnExternalModeExecution(bool)` callback with a synchronous, error-returning hook
`BeforeExternalServerStart() error`, gated by `StartServer` in
[management.go:28-39](../management.go#L28-L39).

That PLAN was incomplete: it only covered **one** path that transitions to
external mode (`StartServer` re-entering with `web/server.go` present). It did
not consider [switch_mode.go:CreateTemplateServer](../switch_mode.go), which is
a **separate entry point** that:

1. Stops the in-memory server.
2. Generates `web/server.go` from an embedded template.
3. Switches `h.executionInternal = false` and replaces `h.strategy` with
   `newExternalStrategy(h)`.
4. Calls `s.Start(nil)` directly — **bypassing `StartServer`**, and therefore
   bypassing `BeforeExternalServerStart`.

This is the same defect the original PLAN was designed to fix: external mode
boots without giving the orchestrator a chance to flush in-memory state. The
fix must apply symmetrically to every transition path, not only one.

The CI failure that blocked the previous merge (`TestCreateTemplateServerGeneratesFile`
timing out) exercises exactly this path. Closing the gap is the unblocker.

## Root cause (this gap)

[switch_mode.go:38-45](../switch_mode.go#L38-L45):

```go
h.log("Starting External Server...")
// Start the new external server (compiles and runs)
// We pass nil for wg because this is a runtime transition, not application startup
if err := s.Start(nil); err != nil {
    return errors.Join(errors.New("failed to start external server"), err)
}
```

No hook invocation. The orchestrator's `FlushToDisk` (and equivalent
client/asset preparation) never gets a chance to run before the external
binary opens the socket.

## Breaking-free fix

### 1. Invoke `BeforeExternalServerStart` in `CreateTemplateServer` before `s.Start(nil)`

Modify [switch_mode.go:38-45](../switch_mode.go#L38-L45):

```go
h.log("Starting External Server...")

// Gate the external strategy on the same hook that StartServer uses.
// CreateTemplateServer is an external-mode transition entrypoint; the hook
// must fire here too, by the same idempotency contract documented in
// SetBeforeExternalServerStart.
if err := h.BeforeExternalServerStart(); err != nil {
    return errors.Join(errors.New("BeforeExternalServerStart failed"), err)
}

if err := s.Start(nil); err != nil {
    return errors.Join(errors.New("failed to start external server"), err)
}
```

If the hook returns error, `CreateTemplateServer` returns the joined error and
the caller (TUI / orchestrator) sees the failure synchronously. No partial
state — `executionInternal` and `strategy` were already mutated before this
point, but the external process never starts.

> **Note on partial state:** the current `CreateTemplateServer` already mutates
> strategy state before `s.Start` is called. If `s.Start` fails today, the
> handler is left "switched but not started". The hook error path inherits
> that behavior — not made worse. Cleaning up that asymmetry is **out of
> scope** for this PLAN; tracked separately.

### 2. Update the public contract documentation

[docs/SERVER_INTERFACE.md](SERVER_INTERFACE.md) — add to the description of
`SetBeforeExternalServerStart`:

> The hook is invoked synchronously before any transition into external mode,
> including both `StartServer` (when an external server file is detected) and
> `CreateTemplateServer` (when the template generator runs). It is NOT invoked
> by `RestartServer` (which only restarts an already-external strategy).

### 3. Test coverage

Reuse existing test file [test/before_external_hook_test.go](../test/before_external_hook_test.go).
Add:

```go
// CreateTemplateServer must invoke BeforeExternalServerStart before strategy.Start.
func TestCreateTemplateServer_InvokesBeforeHook(t *testing.T) {
    // 1. Build a ServerHandler wired with a tmp AppRootDir and embedded template.
    // 2. Register a hook that records its invocation time.
    // 3. Capture strategy.Start time via a mock strategy injected post-switch
    //    (or by wrapping newExternalStrategy via reflection, mirroring the
    //    existing test helpers).
    // 4. Call h.CreateTemplateServer() (in a goroutine, signal ExitChan to release).
    // 5. Assert hookTime <= startTime AND hookCount == 1.
}

// CreateTemplateServer must propagate hook errors and NOT start the strategy.
func TestCreateTemplateServer_HookErrorAbortsStart(t *testing.T) {
    // 1. Register a hook returning errors.New("boom").
    // 2. Call h.CreateTemplateServer().
    // 3. Assert: returned error wraps "boom", strategy.Start was NOT called.
}
```

### 4. Address the existing `TestCreateTemplateServerGeneratesFile` flakiness

The test's deadlock signature in the CI run (`externalStrategy.Start` blocked
on `chan receive` while the test's non-blocking `case h.ExitChan <- true:
default:` lost the signal) is **pre-existing** — not introduced by this PLAN
or its predecessor. The agent must NOT fix the deadlock as part of this PLAN.
Instead:

- Verify on the unchanged main branch whether the test is flaky.
- If flaky on main: add a `t.Skip("flaky — tracked in #<issue>; see PLAN.md
  §4")` with a TODO comment pointing here, so the merge is not blocked by an
  orthogonal pre-existing failure.
- If passing on main but failing only on this branch: investigate timing
  interaction with the new hook call (none expected — the hook in tests is a
  no-op closure registered before `CreateTemplateServer` runs).

Document the outcome in the PR description.

## Out of scope

- Cleanup of `CreateTemplateServer` partial-state mutations on failure.
- Refactor of `externalStrategy.Start` blocking semantics or ExitChan usage.
- Any change to `client`, `assetmin`, or `app` packages.

## Acceptance criteria

1. [switch_mode.go](../switch_mode.go) `CreateTemplateServer` invokes
   `BeforeExternalServerStart()` before `s.Start(nil)` and propagates its error.
2. The two new tests in §3 pass.
3. `TestCreateTemplateServerGeneratesFile` either passes or is skipped with a
   documented reason per §4.
4. `go test ./test/` is green.
5. `SERVER_INTERFACE.md` reflects the symmetric hook contract.
