> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Part of the orchestrator: `../../docs/HOT_RELOAD_MASTER_PLAN.md` (Phase E) and this module's local orchestrator `docs/PLAN.md`. Depends on `gobuild/docs/PLAN.md` (Phase A) and `gorun/docs/PLAN.md` (Phase B) being merged first. Dispatch after `PLAN_ROUTER_REFACTOR.md` to avoid touching `strategies.go`/`server.go` in overlapping PRs.

# server — fix silent no-compile-but-reload bug + consume Compiler/Runner interfaces

## Problem (confirmed by code inspection, `server/strategies.go`)

```go
// strategies.go:433-445
func (s *externalStrategy) HandleFileEvent(fileName, extension, filePath, event string) error {
    if event == "write" {
        s.handler.log("Go file modified, restarting external server ...")
        err := s.Restart()
        ...
        return err
    }
    return nil   // <-- BUG: any non-"write" event returns nil (success)
}
```

`devwatch` treats a `nil` return from `NewFileEvent`/`HandleFileEvent` as
"handler succeeded" and schedules a browser reload. Editors that perform an
atomic save (write to a temp file, then `rename` over the original — common
on Linux with vim, some JetBrains/VSCode configurations) emit fsnotify ops
other than a plain `write` (`create`/`rename`). When that happens today:
`web/server.go` changes on disk, **no recompile happens**, and the browser
still reloads — serving the **old** server binary. This is the single most
concrete, reproducible instance of "reload without build" in the whole
pipeline.

Additionally, `externalStrategy` (`strategies.go:247-248`) holds
`goCompiler *gobuild.GoBuild` and `goRun *gorun.GoRun` as concrete types,
blocking fast unit tests of `HandleFileEvent`'s branching logic — today's
`server/test/*` suite is integration-only (real compiles, real processes),
so this exact bug has no fast regression test guarding it.

## Required change

1. Change `event == "write"` to treat `write`, `create`, and `rename`
   uniformly as "content changed, must recompile" — matching how
   `client.WasmClient`'s file-event handling already treats these ops
   (confirm exact set by reading `client/file_event.go`'s event handling
   before finalizing; do not invent a broader set than what `client` uses,
   the two must be symmetric). Define the accepted set as a named
   `[]string` constant/var (e.g. `var contentChangeEvents = []string{"write", "create", "rename"}`) shared or duplicated consistently with `client`'s
   equivalent — no inline string comparisons scattered across branches.
2. For any event **not** in that set (e.g. `"remove"`), return an explicit
   sentinel error (per the resolved decision: no silent `nil`), e.g.
   `var ErrUnsupportedEvent = errors.New("server: unsupported file event, no rebuild triggered")`,
   so `devwatch` does **not** schedule a reload for it. Confirm with the
   `devwatch` plan (Phase F) that returning a non-nil error here is
   correctly interpreted as "no action taken, don't reload" rather than
   "handler error, log loudly" — align the contract explicitly; if
   `devwatch`'s gate needs to distinguish "no-op" from "real error", add a
   sentinel devwatch can check via `errors.Is`.
3. Change `goCompiler`/`goRun` field types from `*gobuild.GoBuild`/
   `*gorun.GoRun` to `gobuild.Compiler`/`gorun.Runner` (interfaces added in
   Phases A/B). `newExternalStrategy` (around `strategies.go:297-301`)
   keeps constructing real instances — no behavior change for production
   wiring, only the field types change so tests can inject fakes.

## Tests required

Add `server/handle_file_event_test.go` (fast, no real compiler/process,
using `gobuild.FakeCompiler` + `gorun.FakeRunner`):

1. `HandleFileEvent(..., event="write")` with `FakeCompiler.CompileErr=nil`
   → assert `FakeCompiler.CompileCallCount == 1`, `FakeRunner.RunCallCount == 1`, returns `nil`.
2. Same with `event="create"` and `event="rename"` → same assertions (the
   fix for the core bug).
3. `event="remove"` → assert `FakeCompiler.CompileCallCount == 0` and a
   non-nil sentinel error is returned (no silent success).
4. `FakeCompiler.CompileErr = someErr`, `event="write"` → assert
   `FakeRunner.RunCallCount == 0` (must not start the old/broken binary) and the error propagates unchanged.

## Constraints

- No hardcoded strings — event names (`"write"`, `"create"`, `"rename"`,
  `"remove"`) must be named constants if not already defined somewhere in
  `devwatch` that this package could import instead of redefining. Check
  `devwatch` for existing exported event constants before adding new ones
  here — don't duplicate.
- Must not break existing `server/test/*` integration suite
  (`restart_on_fix_test.go`, `restart_cleanup_test.go`,
  `startserver_integration_test.go`, `startserver_blackbox_test.go`,
  `before_external_hook_test.go`, `modes_test.go`, `port_conflict_test.go`,
  `https_test.go`) — these keep running real compiles; only the new test
  file uses fakes.
- Run `gotest ./...` in `server/` after the change.

## Stages

| Stage | Description | Output |
|---|---|---|
| 1 | Check `devwatch` and `client` for existing event-name constants to reuse; define shared set if none exist | Confirmed constant source |
| 2 | Change `HandleFileEvent` to treat write/create/rename uniformly, return sentinel error otherwise | `server/strategies.go:433-445` |
| 3 | Change `externalStrategy.goCompiler`/`goRun` field types to interfaces | `server/strategies.go:247-248` and constructor |
| 4 | Add `server/handle_file_event_test.go` with the 4 fast cases above | New test file, passing |
| 5 | Run full `server` suite (integration + new fast tests), confirm no regressions | Test output attached to PR |
