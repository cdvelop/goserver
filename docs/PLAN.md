---
PLAN: "feat!: finish the generated server main — wire strategy selection, compile the artifact, HTTPS-by-default with LAN certs"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — finish PR #17 (the server main as a build artifact)

## Why this plan exists

PR #17 (`feat!: generate server main as build artifact`, merged into this
branch's history) did **part** of the previous plan and left the feature
non-functional. This plan finishes it. Do **not** re-do the parts already done;
do **all** the parts listed as missing below.

### Already done by PR #17 — DO NOT redo, DO NOT revert

- `maingen.go` exists with `GenerateMain(rootDir, modulePath string, cfg MainConfig) (string, error)`,
  `MainConfig{Port, PublicDir, DevTLS}`, `HasRoutes(rootDir string) bool`, the
  `text/template` for the generated `main.go`, and an AST helper
  `detectRegisterArgs` that reads the arity of `routes.Register`.
- `generator.go`, the `templates/` directory and the `//go:embed templates/*`
  directive are deleted. `generateServerFromEmbeddedMarkdown` /
  `getExpectedServerContent` are gone.
- `switch_mode.go` and `server.go` call `GenerateMain(...)` (instead of the old
  markdown generator) when `<AppRootDir>/<SourceDir>/<MainInputFile>` is absent.
- `httpd/adapter.go` gained `Mount` / `prefixRouter`. Keep it.

### Missing / broken — THIS PLAN

1. **`GenerateMain` writes `.build/server/main.go`, but nothing compiles that
   path.** `newExternalStrategy` in `strategies.go:331` still hard-codes the
   compiler input as `filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)`
   (i.e. `web/main.go`). The generated artifact is never built. → **Stage 2, 3**
2. **`HasRoutes` is dead code** — defined, never called. Strategy selection in
   `management.go:15` still keys off `<SourceDir>/<MainInputFile>` only.
   A project that declares `routes/routes.go` and no `web/server.go` never
   reaches external mode. → **Stage 1, 2**
3. **HTTPS is not on by default.** `New()` in `server.go:98` never sets
   `Https: true`. → **Stage 4**
4. **The stale defect comment is still there.** `strategies.go:127-133` still
   says *"Strategies.go is in package server, so we can't easily call
   httpd.getOrCreateDevCert unless we export it"*. The internal strategy still
   serves plain HTTP even when `Https` is set. → **Stage 4, 5**
5. **The dev certificate covers only `localhost` / `127.0.0.1` / `::1`.**
   `httpd/devcert.go` has no `net.Interfaces()` enumeration, so a phone hitting
   `https://192.168.x.x:<port>` gets a hard TLS rejection. → **Stage 6**
6. **No `CAPath`, no `/__webtyp/ca` handler, no `DevCertSPKI`.** → **Stage 7, 8**
7. **Hardcoded strings in `maingen.go`** (`"go.mod"`, `"module "`, `"Register"`,
   `"s.Router()"`, `"nil"`, `0755`, `0644`) violate the no-literals rule; and
   the `.gitignore` write is an ad-hoc `ensureGitIgnore` instead of the
   `ServerHandler.SetGitIgnoreAdd` hook. → **Stage 1**
8. **`go.mod` still requires `webtyp.com/markdown`** (now unused). → **Stage 9**
9. **No tests** for `GenerateMain`, `HasRoutes`, arity detection, the
   `.gitignore` behaviour, `DevCertSPKI`, LAN SANs, or `/__webtyp/ca`. → **Stage 10**

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest ./...` for the whole suite and `gotest -run TestName ./...` for
one test. Never call `go test` directly.

## Context the executor has none of — read fully

`webtyp.com/server` runs a user's project in two strategies (`strategies.go`):

- **Internal** (`internalStrategy`, `strategies.go:44`) — an in-process `httpd`
  server that serves compiled assets. Nothing the user wrote runs here.
- **External** (`externalStrategy`, `strategies.go:302`) — compiles a Go `main`
  to a binary and runs it as a child process; restarts it on file events; kills
  it on shutdown. This lifecycle is correct and complete — **only change which
  file it compiles.**

`ServerHandler` (`server.go:62`) fields you will touch:

| Field / method | Meaning |
|---|---|
| `AppRootDir` | project root (has `go.mod`). Default `"."`. |
| `SourceDir` | dir of the user's optional hand-written main, relative to root. Default `"web"`. |
| `OutputDir` | compile/run dir. Default `"web"`. |
| `PublicDir` | static assets dir. Default `"web/public"`. |
| `MainInputFile` / `mainFileExternalServer` | user main filename. Default `"main.go"`. |
| `Https` (`Config.Https`) | HTTPS toggle. Set via `SetHTTPS(bool)`. |
| `GitIgnoreAdd` (`Config.GitIgnoreAdd func(entry string) error`) | set via `SetGitIgnoreAdd`; default no-op. |
| `Port()` | current port (concurrency-safe). |
| `log(...any)` | logger. |

`routescan` (`webtyp.com/router/routescan`, already a dependency):

- `routescan.DefaultFile` = `"routes/routes.go"` (relative to project root).
- `routescan.Scan(rootDir) ([]Decl, error)` — returns `nil, nil` when the file
  is absent (a project without routes is legal).
- **`routescan` does NOT expose the signature/arity of `routes.Register`.** The
  AST helper `detectRegisterArgs` in `maingen.go` stays; this plan only makes it
  greppable-constant-driven and tested. Do not try to get arity from `routescan`.

`httpd` package:

- `httpd.New(httpd.Config) *httpd.Server`; `Server.Router() router.Router`;
  `Server.Handler() (http.Handler, error)`.
- `httpd.TLSConfig{AutoCert, Domain, CertFile, KeyFile, DevTLS bool}` — at most
  one mode; `DevTLS: true` makes `listenAndServe` call the unexported
  `getOrCreateDevCert()` (`httpd/devcert.go`).
- `getOrCreateDevCert() (certFile, keyFile string, err error)` writes
  `~/.webtyp/httpd/certs/localhost.{crt,key}` and best-effort installs the CA in
  the OS truststore via `github.com/smallstep/truststore`.

---

## Design gate

The public surface is **unchanged from the PR #17 plan** — `GenerateMain`,
`MainConfig`, `HasRoutes`, `GeneratedMainDir`, plus two constants and one
function this plan adds that were already specified there: `CAPath` and
`DevCertSPKI`. No new decision. The five answers from the previous plan stand:

1. **Prior art.** Next.js has no server main; SvelteKit generates it per
   adapter; Rails/Django/Phoenix ship a launcher; Go's Encore generates the
   main. Hand-writing `main()` is the exception.
2. **Novice-name test.** Nothing new is user-facing. The generated file sits at
   a stable readable path (`.build/server/main.go`) — generated code, not magic.
3. **Ledger.** Files the developer maintains −1 (`web/server.go` no longer
   generated); concepts −1 (Internal vs External route registration); route sets
   that can diverge −1 (1→0); ways to start a server 0 (escape hatch replaces the
   old default). Last row not positive. ✔
4. **Where it belongs.** Generating and running the project's server process is
   this package's existing job. No new package.
5. **What it deletes.** The last of the markdown-template machinery
   (`webtyp.com/markdown` dependency) and the stale in-package-`server` comment.

---

## Stage 1 — `maingen.go`: constants + `.gitignore` via the hook

### 1a. Replace every string literal in `maingen.go` logic with a named constant

Add to the existing `const` block:

```go
const (
	GeneratedMainDir      = ".build/server"           // already present
	GeneratedMainFilename = "main.go"                 // already present
	GitIgnoreFile         = ".gitignore"              // already present
	BuildDirGitIgnore     = ".build/"                 // already present

	GoModFilename    = "go.mod"
	ModuleDirective  = "module"
	RegisterFuncName = "Register"
	RouterArgExpr    = "s.Router()"
	DependencyArg    = "nil"

	dirPerm  os.FileMode = 0o755
	filePerm os.FileMode = 0o644
)
```

Use them everywhere in `readModulePath`, `GenerateMain`, `detectRegisterArgs`,
`ensureGitIgnore`. No bare `"go.mod"`, `"module "`, `"Register"`, `"s.Router()"`,
`"nil"`, `0755`, `0644` may remain in the file.

Acceptance: `grep -nE '"(go\.mod|module |Register|s\.Router\(\)|nil)"|0o?755|0o?644' maingen.go` → **empty**.

### 1b. `.gitignore` write goes through `ServerHandler.SetGitIgnoreAdd`

`GenerateMain` is a free function and must stay one. Move the `.gitignore`
concern to the caller:

- **Delete** `ensureGitIgnore` from `maingen.go` and its call inside
  `GenerateMain`. `GenerateMain` no longer touches `.gitignore`.
- In **`switch_mode.go`** and **`server.go`**, immediately after a successful
  `GenerateMain(...)` call, add:

  ```go
  if h.GitIgnoreAdd != nil {
      _ = h.GitIgnoreAdd(BuildDirGitIgnore)
  }
  ```

  `SetGitIgnoreAdd`'s callback is responsible for de-duplication (the caller in
  `webtyp.com/app` already appends only if absent) — but also make the default
  no-op path harmless (it is: `func(string) error { return nil }`).

- Keep `BuildDirGitIgnore = ".build/"` (covers the binary too).

### 1c. `detectRegisterArgs` — drive off the constants, keep the AST approach

`detectRegisterArgs(routesFile string) string` already:
- parses `routes/routes.go` with `go/parser`,
- finds `func Register` (or the first func whose first param is `router.Router`),
- returns `RouterArgExpr` when arity ≤ 1,
- returns `RouterArgExpr + ", nil, nil, …"` matching arity otherwise.

Change only: use `RegisterFuncName`, `RouterArgExpr`, `DependencyArg` constants;
on any parse failure return `RouterArgExpr` (single-arg form) — never error out
of `GenerateMain` because of arity detection. Add a doc comment stating that
`routescan` does not report arity and this AST read is the deliberate fallback.

---

## Stage 2 — strategy selection keys off routes, not `web/main.go`

### 2a. `ServerHandler` gets the decision helpers

Add to **`server.go`** (or a new `maindecision.go` in package `server` — your
call, but name it, don't inline in `management.go`):

```go
// usesGeneratedMain reports whether this project's server process is the
// generated artifact (routes/routes.go present) rather than a hand-written main.
func (h *ServerHandler) usesGeneratedMain() bool {
	return HasRoutes(h.AppRootDir) && !h.hasHandWrittenMain()
}

// hasHandWrittenMain reports whether the user supplied their own server main at
// <AppRootDir>/<SourceDir>/<MainInputFile> (canonically web/server.go). It is
// the escape hatch: it wins over generation, and the tool never overwrites it.
func (h *ServerHandler) hasHandWrittenMain() bool {
	_, err := os.Stat(filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer))
	return err == nil
}

// needsExternalProcess reports whether StartServer must run a compiled child
// process (generated main, or the user's own main) rather than the in-memory
// internal server.
func (h *ServerHandler) needsExternalProcess() bool {
	return h.hasHandWrittenMain() || h.usesGeneratedMain()
}

// serverMainRelPath is the compiler input, relative to AppRootDir: the user's
// own main when present, otherwise the generated artifact.
func (h *ServerHandler) serverMainRelPath() string {
	if h.hasHandWrittenMain() {
		return filepath.Join(h.SourceDir, h.mainFileExternalServer)
	}
	return filepath.Join(GeneratedMainDir, GeneratedMainFilename)
}
```

### 2b. `management.go` `StartServer` — decide with `needsExternalProcess()`

Current logic (`management.go:13-27`) stats `<SourceDir>/<MainInputFile>` and
logs `"External mode: found <path>"` / `"Internal mode: <path> does not exist …"`.

Replace it so that:

- `h.needsExternalProcess()` true → if `h.usesGeneratedMain()`, call
  `GenerateMain` (+ `GitIgnoreAdd`) **before** switching strategy (mirror the
  block already in `switch_mode.go` — factor it into one unexported method,
  e.g. `h.ensureServerMain() error`, and call it from all three sites:
  `StartServer`, `CreateTemplateServer`, `SetExternalServerMode`).
  Then `h.executionInternal = false; h.strategy = newExternalStrategy(h)`.
- false → stay internal.

Logging (keep messages greppable, as named constants in `management.go`):

| Condition | Log |
|---|---|
| hand-written main | ``LogExternalHandWritten = "External mode: user server main at"`` + abs path |
| generated main | ``LogExternalGenerated = "External mode: routes/routes.go present — generated main at"`` + abs path |
| neither | ``LogInternalNoRoutes = "Internal mode: no routes/routes.go and no user server main — served from memory"`` |

### 2c. Update `tests/mode_file_decision_test.go`

It currently encodes the **old** rule (external iff `web/server.go` exists) and
its log substrings (`"External mode: found"`, `"Internal mode:"`). Rewrite it to
the new rule:

- `web/server.go` present → external (escape hatch), log contains
  `LogExternalHandWritten`.
- `routes/routes.go` present, no `web/server.go` → external, log contains
  `LogExternalGenerated`.
- neither → internal, log contains `LogInternalNoRoutes`.
- Store still receives **zero** writes in every case (keep
  `TestNoSeEscribeNingunaClaveDeModo`, extended with the routes case).

---

## Stage 3 — compile the generated artifact

In **`strategies.go`**, `newExternalStrategy(h *ServerHandler)`:

- Line ~320: `outName := h.mainFileExternalServer` → derive from
  `h.serverMainRelPath()` instead: `outName := filepath.Base(h.serverMainRelPath())`
  with its extension stripped (keep the existing `filepath.Ext` trim).
- Line ~331: `MainInputFileRelativePath: filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)`
  → `filepath.Join(h.AppRootDir, h.serverMainRelPath())`.
- The `gorun` `WorkingDir` / `OutFolderRelativePath` stay `h.OutputDir` — the
  binary still lands there; only the **source** moves.
- The binary `.gitignore` line (`binaryPath := filepath.Join(h.OutputDir, outName+exe_ext)`)
  stays.

Do not touch `Start` / `Stop` / `Restart` / `HandleFileEvent` / the file-event
restart path — they compile-then-run whatever `MainInputFileRelativePath` points
at, which is now correct for both cases.

---

## Stage 4 — HTTPS on by default

### 4a. `server.go` `New()`

In the `Config` literal in `New()` (`server.go:~104`), add `Https: true`.
Add a one-line comment: `// HTTPS-by-default in dev; SetHTTPS(false) is an explicit opt-out.`

### 4b. `SetHTTPS` doc

Update the `SetHTTPS` doc comment (`server.go:172`) to:
`// SetHTTPS enables or disables HTTPS. The default is true; passing false is an
// explicit, deliberate opt-out (never a silent default, never read from a
// gitignored file).`

### 4c. Internal strategy actually serves TLS

In `strategies.go`, the internal strategy currently builds a raw
`&http.Server{Addr, Handler}` and (see stale comment at line 127-133) never
does TLS. Fix:

- Add to **`httpd`** an exported wrapper (new file `httpd/devcert_export.go` or
  add to `devcert.go`):

  ```go
  // DevCertFiles returns paths to the development certificate and key, creating
  // and truststore-installing them on first call. Same artifact the DevTLS
  // listen path uses.
  func DevCertFiles() (certFile, keyFile string, err error) {
      return (&Server{}).getOrCreateDevCert()
  }
  ```

  `getOrCreateDevCert` only uses `s.log` for a best-effort warning; a
  zero-value `*Server` is fine, but guard the `s.log` call inside
  `getOrCreateDevCert` with `if s.log != nil` (add a nil check — the internal
  strategy has no `httpd.Server`). Verify `Server.log` — if it is a field of
  func type, the nil check is `if s.log != nil`; if it is a method, route the
  warning through a package-level `var devcertLog = func(...any){}` that
  `DevCertFiles` leaves as no-op.

- In `internalStrategy.Start`, replace the dead `if s.handler.Https { … }` block
  (lines 127-133, **delete the comment entirely**) with: when
  `s.handler.Https`, obtain `certFile, keyFile, err := httpd.DevCertFiles()`,
  store them on the strategy, and serve via `s.server.ServeTLS(ln, certFile, keyFile)`
  instead of `s.server.Serve(ln)`. Keep the plain `Serve(ln)` path for
  `Https == false`. `WaitForPortListening` is already called with
  `s.handler.Https`.

- `OpenBrowser` in the internal strategy is currently always called with
  `false` (`strategies.go:~155` `OpenBrowser(s.handler.Port(), false)` with a
  `// Internal server is always http` comment). Change to `s.handler.Https` and
  delete that comment.

### 4d. Generated main + external: DevTLS from `cfg.DevTLS`

The template already renders `TLS: httpd.TLSConfig{DevTLS: {{.DevTLS}}}` and the
callers already pass `DevTLS: h.Https`. Confirm the caller in `management.go`'s
`ensureServerMain` also passes `DevTLS: h.Https`. Nothing else here.

---

## Stage 5 — kill the stale comment (acceptance criterion)

`grep -rn "we can't easily call httpd\|Strategies.go is in package server\|For now, let's keep internalStrategy as is" .` → **empty** after Stage 4c.

---

## Stage 6 — the dev certificate must cover the LAN

In **`httpd/devcert.go`**, `getOrCreateDevCert`:

### 6a. Enumerate host IPv4s, injectably

```go
// interfaceAddrs is net.InterfaceAddrs, swappable in tests.
var interfaceAddrs = net.InterfaceAddrs

// lanIPs returns every non-loopback IPv4 of the host's active interfaces.
func lanIPs() []net.IP {
	var out []net.IP
	addrs, err := interfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			out = append(out, v4)
		}
	}
	return out
}
```

The cert template's `IPAddresses` becomes
`append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, lanIPs()...)`,
`DNSNames` stays `[]string{"localhost"}`. Named constants for `"localhost"`,
`"127.0.0.1"`, `"::1"` (e.g. `dnsLocalhost`, `ipLoopback4`, `ipLoopback6`).

### 6b. Regenerate when the address set changes

Beside `localhost.crt` / `localhost.key`, write `localhost.sans` — a sorted,
newline-joined list of the SAN strings the cert was built with (`localhost`,
each IP). On entry, if `localhost.crt` and `localhost.key` exist **and**
`localhost.sans` matches the current desired set → reuse. Otherwise regenerate
all three. Constant `devCertSANsFilename = "localhost.sans"`.

State this anti-footgun in a comment: *a laptop that changes networks otherwise
serves a cert for an IP it no longer has, and the failure is a bare TLS error.*

---

## Stage 7 — `CAPath` + the `/__webtyp/ca` handler

### 7a. Constant (in `httpd`, since the handler is registered there and in the template)

```go
// CAPath is where the development certificate is served so a phone on the LAN
// can install it. Served DER-encoded with CADownloadContentType.
const CAPath = "/__webtyp/ca"

// CADownloadContentType is the MIME type iOS/Android expect for a CA profile.
const CADownloadContentType = "application/x-x509-ca-cert"
```

Also re-export from package `server` if callers there need it:
`const CAPath = httpd.CAPath` — but prefer callers importing `httpd`.

### 7b. Serve it — internal strategy

After routes are registered on `r` in `internalStrategy.Start` (both the
`len(routes) > 0` and the no-routes branch), and only when `s.handler.Https`:

```go
r.PublicAsset(httpd.CAPath, func(ctx router.Context) {
	der, err := httpd.DevCertDER()
	if err != nil {
		ctx.SetStatus(http.StatusServiceUnavailable)
		return
	}
	ctx.SetHeader("Content-Type", httpd.CADownloadContentType)
	ctx.Write(der)
})
```

`PublicAsset` is correct here (it is an asset, public by construction — see
`AGENTS.md`): never `Get(...).Public()`.

### 7c. Serve it — generated main

Add to the template in `maingen.go`, inside `main()` after
`routes.Register(...)`, gated on `{{.DevTLS}}`:

```go
{{if .DevTLS}}s.Router().PublicAsset(httpd.CAPath, func(c router.Context) {
	der, err := httpd.DevCertDER()
	if err != nil { c.SetStatus(503); return }
	c.SetHeader("Content-Type", httpd.CADownloadContentType)
	c.Write(der)
})
{{end}}```

Add the `webtyp.com/router` import to the template's import list (guard with
`{{if .DevTLS}}` is not possible in an import block — always import it and use
`_ = router.Context(nil)` is ugly; instead **always** render the CA block but
gate its effect: simplest is to always import `router` and always register the
handler, keying the 503 on `DevCertDER` failing when TLS is off). Decide and
keep it compiling either way; the test (Stage 10, test 8) pins the behaviour.

### 7d. `DevCertDER`

```go
// DevCertDER returns the development CA certificate, DER-encoded, generating it
// if necessary.
func DevCertDER() ([]byte, error)
```

Read `localhost.crt`, `pem.Decode`, return `block.Bytes`. Constant for the
`"CERTIFICATE"` PEM type if not already present.

---

## Stage 8 — `DevCertSPKI`

In **`httpd`** (new `httpd/spki.go` or in `devcert.go`):

```go
// DevCertSPKI returns the base64-encoded SHA-256 of the development
// certificate's SubjectPublicKeyInfo — the value Chrome's
// --ignore-certificate-errors-spki-list expects. Used by webtyp.com/devbrowser.
func DevCertSPKI() (string, error) {
	der, err := DevCertDER()
	if err != nil {
		return "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}
```

44-character output for SHA-256 → base64. No new deps (`crypto/sha256`,
`encoding/base64` are stdlib — this is backend tooling, stdlib is correct here,
do **not** swap for `webtyp/fmt`).

---

## Stage 9 — drop the unused dependency

`webtyp.com/markdown` is no longer imported anywhere. Run `go mod tidy` and
commit the resulting `go.mod` / `go.sum` (the `webtyp.com/markdown v0.0.3` line
in `require` disappears).

Acceptance: `grep -rn "webtyp.com/markdown" go.mod` → **empty**;
`grep -rn "webtyp.com/markdown" --include='*.go' .` → **empty**.

---

## Stage 10 — tests

Location rule (`AGENTS.md`): black-box behavioural tests → `tests/` as
`package server_test`. Tests needing unexported identifiers → beside the code
(`package server` at root, `package httpd` in the subpackage).

| # | Test | File | Kind |
|---|---|---|---|
| 1 | `GenerateMain` into `t.TempDir()` (with a `go.mod` `module example.com/app` and a `routes/routes.go`) → returned path exists, parses with `go/parser`, `import` list contains `"example.com/app/routes"` and `"webtyp.com/server/httpd"`. | `tests/maingen_test.go` | black-box (`server.GenerateMain`) |
| 2 | `server.HasRoutes(dir)` → false for an empty temp dir; true after creating `<dir>/routes/routes.go`. | `tests/maingen_test.go` | black-box |
| 3 | `detectRegisterArgs` with `func Register(r router.Router)` → `"s.Router()"`; with `func Register(r router.Router, db *sql.DB, mailer Mailer)` → `"s.Router(), nil, nil"`; with an unparseable file → `"s.Router()"`. | `maingen_argstest.go` (root, `package server`) | white-box (unexported func) |
| 4 | With `<root>/web/server.go` present, `StartServer` selects external **and** `serverMainRelPath()` returns `web/server.go` (not `.build/server/main.go`); `GenerateMain` is not invoked (assert `.build/server/main.go` absent afterwards). | `handle_file_event_test.go` sibling — `maindecision_test.go` (root, `package server`) | white-box |
| 5 | Run the `ensureServerMain`/`GitIgnoreAdd` path twice against one temp project with a stub `SetGitIgnoreAdd` that appends to a slice → `.build/` requested; assert the real `webtyp.com/app` de-dup contract by having the stub only append when absent, then assert the file contains `.build/` exactly once. | `tests/gitignore_buildpath_test.go` | black-box |
| 6 | `httpd.DevCertSPKI()` against a fixed cert fixture (generate one deterministically from a fixed seed / fixed key in the test, write it to the expected path via a `t.Setenv("HOME", tmp)` redirect) → returns a 44-char base64 string, stable across two calls. | `httpd/spki_test.go` (`package httpd`) | white-box (HOME redirect + unexported paths) |
| 7 | Stub `httpd.interfaceAddrs` to return `192.168.1.50/24` + loopback → the generated cert's `IPAddresses` contains `127.0.0.1`, `::1` and `192.168.1.50`; its `DNSNames` contains `localhost`. Changing the stub to a different IP and calling again → cert regenerated (different serial / new IP present, old absent). | `httpd/devcert_test.go` (`package httpd`) | white-box |
| 8 | `GET /__webtyp/ca` against the internal strategy's handler (or a minimal `httpd.New` + the registration helper) with `Https: true` → 200, `Content-Type: application/x-x509-ca-cert`, body is valid DER (`x509.ParseCertificate` succeeds). | `tests/ca_endpoint_test.go` | black-box |

All existing tests must stay green, including the rewritten
`tests/mode_file_decision_test.go` (Stage 2c) and
`tests/generator_test.go` (already adjusted in PR #17 — leave it, just make sure
it still passes with the new selection logic).

---

## Constraints

- **No hardcoded strings.** Paths, filenames, log messages, MIME types, PEM
  types, SAN literals → named constants in the package that owns them.
- **`GenerateMain` stays a free function**; the `.gitignore` side effect belongs
  to the `ServerHandler` caller via `SetGitIgnoreAdd`.
- **Backend tooling uses the standard library.** `go/ast`, `go/parser`,
  `crypto/*`, `encoding/*`, `net` are correct here. Do **not** replace them with
  `webtyp/fmt` / `webtyp/json` — this package never compiles to WASM.
- **Do not rewrite the External lifecycle.** Only the compiler input path
  changes.
- **Never overwrite a user's `web/server.go`.** The escape hatch always wins and
  is greppable (a file exists or does not), never a config flag.
- **No `cmd/` in this package.** Callers stay thin.

## Acceptance criteria

1. `grep -rn "server_basic\|generateServerFromEmbeddedMarkdown\|getExpectedServerContent" --include='*.go' .` → empty. *(already true; keep it true)*
2. `ls templates/ 2>/dev/null` → nothing. *(already true)*
3. `grep -rn "we can't easily call httpd\|Strategies.go is in package server\|Internal server is always http" .` → empty.
4. `grep -rn "webtyp.com/markdown" .` → empty (no `go.mod`, no `.go`).
5. `grep -rn "HasRoutes\|needsExternalProcess\|serverMainRelPath" --include='*.go' . | grep -v _test` → at least one **call** site each in `management.go` / `strategies.go` (not just the definition).
6. A temp project with `routes/routes.go` and no `web/server.go` → `StartServer`
   picks external, `GenerateMain` writes `.build/server/main.go`, and that file
   is what `newExternalStrategy` compiles.
7. `gotest ./...` → race ✅, tests ✅, all 8 new tests present and passing.
8. `go build ./... && go vet ./...` → clean.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 1 | constants + `.gitignore` via hook + `detectRegisterArgs` cleanup | `maingen.go`, `switch_mode.go`, `server.go` | test 3; criterion (grep literals) |
| 2 | selection helpers + `management.go` decision + decision-test rewrite | `server.go`/`maindecision.go`, `management.go`, `tests/mode_file_decision_test.go` | tests 2, 4; criteria 5, 6 |
| 3 | compile the generated artifact | `strategies.go` | criterion 6; fixture builds |
| 4 | HTTPS by default + internal-strategy TLS + `DevCertFiles` | `server.go`, `strategies.go`, `httpd/devcert.go` | builds; browser opens https |
| 5 | delete stale comment | `strategies.go` | criterion 3 |
| 6 | LAN SANs + regenerate-on-change | `httpd/devcert.go` | test 7 |
| 7 | `CAPath` + `/__webtyp/ca` + `DevCertDER` | `httpd/devcert.go`, `httpd/ca.go`, `maingen.go` | test 8 |
| 8 | `DevCertSPKI` | `httpd/spki.go` | test 6 |
| 9 | `go mod tidy` | `go.mod`, `go.sum` | criterion 4 |
| 10 | all remaining tests | `tests/*`, `*_test.go` beside code | tests 1, 5; criterion 7 |

Sequential. Stages 6-8 may be done together (all in `httpd`).
