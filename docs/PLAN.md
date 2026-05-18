# PLAN: Fix publicDir — Pass Absolute Path via Argument from tinywasm/app

## Development Rules

- Generated server code (`server_basic.md` template) must NOT assume a working directory.
- Template variables (`{{.PublicDir}}`, `{{.AppPort}}`) are **compile-time** values baked at code generation. Runtime topology (CWD, absolute paths) belongs to `tinywasm/app`.
- `lookupArg` is the standard mechanism to receive runtime arguments. Follow the `-server_port` precedent for any new runtime parameter.
- Never use `filepath.Abs` in the generated server — absolute paths must come from the caller (`tinywasm/app`), not be computed from an unknown CWD.

---

## Root Cause

### Architectural conflict introduced in commit `3519fe2`

| Responsibility | Location | Value |
|---------------|----------|-------|
| Template default `PublicDir` | `tinywasm/server` Config default | `"web/public"` (relative to **project root**) |
| Binary working directory (CWD) | `tinywasm/app` (`OutputDir = WebDir`) | `<root>/web/` |

The template hardcodes `publicDir := "{{.PublicDir}}"` → `"web/public"` at generation time.  
At runtime the binary runs from `<root>/web/`, so `"web/public"` resolves to `<root>/web/web/public` — a path that does not exist.

**Result:** every HTTP request returns 404. The gzip middleware compresses the 404 body but the browser never receives `Content-Encoding: gzip` → garbage characters rendered in the DOM.

### Why the previous approach was architecturally correct

The old implementation used `-public-dir` flag + `filepath.Abs`. The flag was a **runtime argument** passed by the caller. The caller knew the absolute path; the binary just received it. Removing the flag reception broke this contract.

### Why the argument is the right fix (not fixing the hardcoded path)

1. **`server_port` precedent** — `tinywasm/app` already passes `-server_port=6060` because port is a runtime deployment decision the template cannot know. `publicDir` is identical: it depends on where the binary is executed from, which only `tinywasm/app` controls.

2. **Compile-time vs runtime separation** — `{{.PublicDir}}` is baked into source at generation time. The binary's CWD is determined at runtime by `tinywasm/app`. These are different lifecycle stages; conflating them causes path breakage.

3. **Future-proof against `OutputDir` changes** — if `tinywasm/app` ever changes where the binary runs from, all generated servers would silently break. With an absolute path argument, the binary is CWD-agnostic.

4. **`tinywasm/app` already has the absolute path** — `filepath.Join(h.RootDir, h.Config.WebPublicDir())` (app/section-build.go:46) is exactly the value needed. It just needs to be forwarded.

---

## Fix Plan

### Part 1 — `tinywasm/server`: add `server_public_dir` argument to template

**File:** `templates/server_basic.md`

Add `lookupArg("server_public_dir")` with fallback to the hardcoded template default:

```go
func main() {
    port := lookupArg("server_port")
    if port == "" {
        port = "{{.AppPort}}"
    }

    publicDir := lookupArg("server_public_dir")
    if publicDir == "" {
        publicDir = "{{.PublicDir}}"
    }

    log.Printf("Serving static files from: %s on port %s", publicDir, port)
    // ... rest unchanged
}
```

**Why the fallback:** preserves backward compatibility. If `tinywasm/app` doesn't pass the arg (older version), the hardcoded default is used. The default will still be wrong in most cases, but the arg takes precedence when provided.

### Part 2 — `tinywasm/app`: pass absolute publicDir as argument

**File:** `app/section-build.go`, `ArgumentsToRunServer` (line ~116)

```go
// Before (only port):
return []string{"-server_port=" + h.Config.ServerPort()}

// After (port + absolute public dir):
return []string{
    "-server_port=" + h.Config.ServerPort(),
    "-server_public_dir=" + filepath.Join(h.RootDir, h.Config.WebPublicDir()),
}
```

`filepath.Join(h.RootDir, h.Config.WebPublicDir())` is already computed on line 46 of `section-build.go` — reuse it or extract it to a method.

### Part 3 — Regenerate goflare-demo server binary

The `goflare-demo/web/server.go` was generated from the old template (without `server_public_dir` support). After Part 1, delete the generated file so `tinywasm/server` regenerates it with the new template.

```bash
rm goflare-demo/web/server.go
# tinywasm will regenerate on next project start
```

### Part 4 — Verify fix in browser

```bash
curl -sv http://localhost:6060/ 2>&1 | grep -E "HTTP/|Content-"
# Expected: HTTP/1.1 200 OK, Content-Type: text/html

curl -sv -H "Accept-Encoding: gzip" http://localhost:6060/style.css 2>&1 | grep -E "HTTP/|Content-"
# Expected: HTTP/1.1 200 OK, Content-Encoding: gzip
```

Use `mcp__tinywasm__browser_screenshot` to confirm the contact form renders without garbage characters.

---

## Files to Change

| Library | File | Change |
|---------|------|--------|
| `tinywasm/server` | `templates/server_basic.md` | Add `lookupArg("server_public_dir")` with fallback |
| `tinywasm/app` | `app/section-build.go` | Add `-server_public_dir=<abs_path>` to `ArgumentsToRunServer` |
| `goflare-demo` | `web/server.go` | Delete → regenerated by tinywasm/server with new template |

---

## Secondary Issue: Gzip `Content-Encoding` missing for error responses

Observed: 404 responses have a gzip-compressed body (43 bytes) but no `Content-Encoding: gzip` header → browser renders garbage.

This is a separate bug in the gzip middleware (`defer gz.Close()` timing vs Go HTTP response finalization). It is masked by the publicDir bug (once paths are correct, valid files return 200 where gzip works). Tracked separately — do not block the publicDir fix on this.
