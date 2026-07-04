# server — Plan orchestrator index

Two independent, self-contained plans are pending for this module. Neither has
been dispatched/executed yet (confirmed: `strategies.go`/`server.go` still
import `net/http`, no `github.com/tinywasm/router` dependency in `go.mod`,
and `HandleFileEvent`'s event-gating bug is still present as of this write).

| Order | Plan | Scope |
|---|---|---|
| 1 | [PLAN_ROUTER_REFACTOR.md](PLAN_ROUTER_REFACTOR.md) | Migrate `server`'s route-registration surface from `net/http` to `tinywasm/router`. Older, already in progress before the hot-reload investigation. |
| 2 | [PLAN_HOTRELOAD.md](PLAN_HOTRELOAD.md) | Fix the fsnotify event-gating bug in `externalStrategy.HandleFileEvent` (silent reload-without-compile) + adopt `gobuild.Compiler`/`gorun.Runner` interfaces. Part of `../../docs/HOT_RELOAD_MASTER_PLAN.md` (Phase E). |

## Why sequential, not parallel

Both plans touch `server/strategies.go` and `server/server.go`. Dispatching
them in parallel risks merge conflicts on the same functions
(`externalStrategy`, its constructor, `HandleFileEvent`). Dispatch
`PLAN_ROUTER_REFACTOR.md` first since it predates this investigation; once
merged, dispatch `PLAN_HOTRELOAD.md`. `PLAN_HOTRELOAD.md` itself still
depends on `gobuild/docs/PLAN.md` and `gorun/docs/PLAN.md` (Phases A/B of
the hot-reload master plan) being merged first, independent of the router
refactor's timing — if the router refactor stalls, `PLAN_HOTRELOAD.md` can
still be dispatched on its own once A/B are ready; it does not require the
router refactor to be done first, only recommended to avoid conflicts.

## How to dispatch

Per the `agents-workflow` skill, `codejob` only reads `docs/PLAN.md`. To
dispatch one of the two plans above: copy its content into `docs/PLAN.md`
(replacing this index temporarily), run `codejob`, and once merged restore
this index file with the remaining plan copied in, or ask Claude to do the
swap for you.
