# PLAN: Simplify Default Server Template — Manual Arg Parsing

## Problem

The default server template uses Go's `flag` package. `flag.Parse()` calls `os.Exit(2)` on
any unknown flag. Every new flag that `app` or `client` passes to the server binary breaks
existing generated servers.

Past breakages:
- `-dev` added in `app/section-build.go` → server crashed
- `-wasmsize_mode` added in `client/javascripts.go` → server crashed

## Root Cause

`flag` package is the wrong tool here. The server only needs **one** value from its caller:
the port. Everything else is irrelevant to a static-file server.

## Solution — Manual `os.Args` scan, no `flag` package

Replace `flag` with a small helper that scans `os.Args` for a specific key and returns the
value. Unknown args are silently ignored.

```go
// lookupArg returns the value for -key=value or -key value in os.Args.
// Returns "" if not found.
func lookupArg(key string) string {
    prefix := "-" + key + "="
    for i, arg := range os.Args[1:] {
        if strings.HasPrefix(arg, prefix) {
            return strings.TrimPrefix(arg, prefix)
        }
        if arg == "-"+key && i+1 < len(os.Args[1:]) {
            return os.Args[i+2]
        }
    }
    return ""
}
```

### New template logic (simplified)

```go
func main() {
    port := lookupArg("server_port")
    if port == "" {
        port = "6060"
    }
    // serve web/public on port
}
```

**No `flag` import. No `-public-dir`. No `-dev`. No `-wasmsize_mode`. No breakage.**

`public-dir` is removed as a flag — the server always serves from `web/public` relative to
its working directory (already set by `gorun` via `WorkingDir`). That is sufficient for the
default case; users who need a custom dir can modify their own `server.go`.

## Files to Change

| File | Change |
|------|--------|
| `server/templates/server_basic.md` | Replace `flag` block with `lookupArg`, only read `-server_port` |
| `goflare-demo/web/server.go` | Same patch (existing generated copy) |

## What Does NOT Change

- `gorun` — no changes needed, already passes `RunArguments`
- `app/section-build.go` — see companion `app/docs/PLAN.md`
- `client/javascripts.go` — `-wasmsize_mode` can still be passed; server ignores it

## Stage Checklist

- [ ] Update `server/templates/server_basic.md`
- [ ] Update `goflare-demo/web/server.go`
- [ ] Run `gotest` in `tinywasm/server`
- [ ] Publish with `gopush`
