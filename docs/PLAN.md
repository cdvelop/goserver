# PLAN: Replace `OnExternalModeExecution` with synchronous `BeforeExternalServerStart` hook

> Status: Ready for execution. Breaking change. No backwards compatibility shims.
> Part of a coordinated multi-package refactor. See orchestrator:
> [tinywasm/app/docs/PLAN.md](../../app/docs/PLAN.md).

## Bug being fixed (in this package)

[server/management.go:28-30](../management.go#L28-L30) invokes
`OnExternalModeExecution(isExternal)` and then **immediately** proceeds to
`strategy.Start(wg)`:

```go
h.OnExternalModeExecution(!isInternal)
if err := strategy.Start(wg); err != nil { ... }
```

The callback signature is `func(bool)` — fire-and-forget. There is no error path
and no ordering guarantee. Any orchestration the caller wires (disk flush,
asset persistence, WASM compile) races with the external binary opening the
socket. In `tinywasm/app` this causes loss of in-memory assets when switching
to external mode (see [app PLAN](../../app/docs/PLAN.md)).

This is a contract bug in `tinywasm/server`: the package offers no way to gate
the external-mode startup on caller-supplied work.

## Breaking redesign

### 1. New hook signature

Replace the fire-and-forget callback with a synchronous, error-returning hook.

```go
// In server.go Config:
BeforeExternalServerStart func() error
```

Setter:

```go
// SetBeforeExternalServerStart registers a function invoked synchronously
// BEFORE strategy.Start in every external-mode StartServer call. Returning
// a non-nil error aborts the transition: strategy.Start is NOT invoked and
// the error is logged.
//
// Idempotency: the hook fires on every external-mode StartServer, not only
// on the internal→external transition (external mode is sticky, persisted
// via the Store). Implementations must be safe to invoke N times.
//
// RestartServer does NOT invoke this hook. See §3.
func (h *ServerHandler) SetBeforeExternalServerStart(fn func() error) *ServerHandler
```

### 2. Call site

[server/management.go:28-32](../management.go#L28-L32) becomes:

```go
if !isInternal {
    if err := h.BeforeExternalServerStart(); err != nil {
        h.log("BeforeExternalServerStart failed, aborting transition:", err)
        return
    }
}
if err := strategy.Start(wg); err != nil { ... }
```

Internal mode skips the hook entirely.

### 3. `RestartServer` bypasses the hook (by design)

[server/management.go:65-67](../management.go#L65-L67) delegates to
`strategy.Restart` and must **not** invoke the hook. Rationale: a restart in
external mode does not need re-orchestration because the caller's previous
hook execution already left the filesystem consistent. Re-firing on every
restart would also be a footgun (double-flushes, double-compiles).

This decision is contractual; the agent must not "fix the asymmetry" by
adding the hook to `Restart`.

### 4. Delete the old API

Delete `OnExternalModeExecution` (field, default, setter) entirely. No alias,
no deprecation shim. Consumers will not compile until they migrate to
`SetBeforeExternalServerStart` — which is the intended forcing function.

### 5. Documentation update

[docs/SERVER_INTERFACE.md](SERVER_INTERFACE.md) — replace the row referencing
`SetOnExternalModeExecution(fn func(bool))` with the new hook signature and
its idempotency / `RestartServer` contracts.

## Tests

Reproducer skeleton already committed at
[../test/before_external_hook_test.go](../test/before_external_hook_test.go)
(skipped with `t.Skip` until the API exists). Coverage:

| Test                                              | What it asserts                                                              |
|---------------------------------------------------|------------------------------------------------------------------------------|
| `TestStartServer_BeforeHookRunsBeforeStrategy`    | External mode invokes the hook before `strategy.Start`.                      |
| `TestStartServer_BeforeHookErrorAborts`           | Non-nil error prevents `strategy.Start` and is logged.                       |
| `TestStartServer_InternalModeSkipsBeforeHook`     | Internal mode does NOT invoke the hook.                                      |
| `TestRestartServer_DoesNotInvokeHook`             | `RestartServer` bypasses the hook by design.                                 |

Agent task: remove `t.Skip` and adapt to the implemented API.

## Out of scope

- Strategy implementations (internal / external) — unchanged.
- Port management, store persistence, UI/Logger plumbing — unchanged.
- Any orchestration logic that *uses* the hook lives in the consuming package
  (see [app PLAN](../../app/docs/PLAN.md)).

## Acceptance criteria

1. `OnExternalModeExecution` and `SetOnExternalModeExecution` are gone from
   the public API (no aliases).
2. `BeforeExternalServerStart` exists with the signature in §1.
3. `management.go` invokes the hook before `strategy.Start` in external mode
   and aborts on error.
4. `RestartServer` does not invoke the hook.
5. `SERVER_INTERFACE.md` is updated.
6. The four tests above pass.
7. `go test ./...` under `server/` is green.
