# PLAN: Fix `-dev` Flag Missing in Default Server Template

## Bug Report

When `tinywasm/app` runs in `DevMode`, it passes `-dev` as a runtime argument to the compiled external server binary (`app/section-build.go:122`). However, the default server template (`templates/server_basic.md`) does not define this flag, causing the server to exit with:

```
flag provided but not defined: -dev
exit status 2
```

Any project using the auto-generated `web/server.go` (e.g. `goflare-demo`) is broken in dev mode.

## Root Cause

**Two-library contract mismatch:**

| Side | Location | What it does |
|------|----------|--------------|
| Caller (`app`) | `app/section-build.go:121-123` | Appends `-dev` to `SetRunArgs` when `h.DevMode == true` |
| Callee (`server`) | `templates/server_basic.md` | Generated template never defines `-dev` flag |

The `app` package assumes any external server it manages will accept `-dev`. The template (which is the **only** auto-generated server) does not fulfil that assumption.

## Fix

### Stage 1 — Update template (`server/templates/server_basic.md`)

Add `flag.Bool("dev", false, "Run in development mode")` to the flag definitions in `main()`.

The flag value can be used by the server to:
- Disable HTTP response caching (already done unconditionally — can be made conditional)
- Log verbosely in dev mode (optional)

Minimal fix (just accept the flag without behavior change):

```go
// existing flags
publicDir := flag.String("public-dir", "", "Directory containing static files")
port      := flag.String("port", "", "Port to listen on")
dev       := flag.Bool("dev", false, "Run in development mode")  // ADD THIS
flag.Parse()
_ = dev  // consumed by orchestrator — accepted but not required for basic server
```

### Stage 2 — Update existing generated files

Projects that already have a generated `web/server.go` (like `goflare-demo`) will **not** be regenerated automatically (generator skips existing files, see `generator.go:66`). Those files must be manually patched or users must delete and regenerate.

Document this in a migration note or add a version-check mechanism in the generator.

### Stage 3 — Add test coverage

Add a test in `server/test/` that:
1. Uses the template content (via `CreateTemplateServer`)
2. Compiles and runs it with `-dev` flag
3. Verifies no `flag provided but not defined` error

## Files to Change

- `server/templates/server_basic.md` — add `-dev` flag definition
- `goflare-demo/web/server.go` — patch existing generated copy (one-time)

## Affected Libraries

- `github.com/tinywasm/server` — owns the template (fix here)
- `github.com/tinywasm/app` — passes the flag (see companion PLAN in `app/docs/PLAN.md`)
