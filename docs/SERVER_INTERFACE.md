# Base B: tinywasm/server — Set* API + ServerInterface Conformance

> **Goal**: Make `server.New()` zero-arg. Replace `Config` struct fields with Set*
> methods for all optional config. Add `RegisterRoutes` to match the assetmin pattern.
> Add a compile-time assertion to guarantee `ServerHandler` satisfies the interface.

---

## API Changes

### `server.New()` — zero-arg constructor

```go
// Before:
func New(c *Config) *ServerHandler

// After:
func New() *ServerHandler
// Config struct is kept internally but no longer required by callers.
// All fields default to sensible values (see table below).
```

### Default values

| Field | Old default | New default |
|---|---|---|
| `AppRootDir` | `"."` | `"."` |
| `SourceDir` | `"web"` | `"web"` |
| `OutputDir` | `"web"` | `"web"` |
| `PublicDir` | `"web/public"` | `"web/public"` |
| `MainInputFile` | `"main.go"` | `"main.go"` |
| `AppPort` | `"8080"` | **`"6060"`** |
| `ExitChan` | `make(chan bool)` | `make(chan bool)` |
| All callbacks | `noop` | `noop` |

### Set* methods — all return `*ServerHandler` for optional chaining

```go
func (h *ServerHandler) SetAppRootDir(dir string) *ServerHandler
func (h *ServerHandler) SetSourceDir(dir string) *ServerHandler
func (h *ServerHandler) SetOutputDir(dir string) *ServerHandler
func (h *ServerHandler) SetPublicDir(dir string) *ServerHandler
func (h *ServerHandler) SetMainInputFile(name string) *ServerHandler
func (h *ServerHandler) SetPort(port string) *ServerHandler
func (h *ServerHandler) SetHTTPS(enabled bool) *ServerHandler
func (h *ServerHandler) SetLogger(fn func(...any)) *ServerHandler  // replaces SetLog()
func (h *ServerHandler) SetExitChan(ch chan bool) *ServerHandler
func (h *ServerHandler) SetOpenBrowser(fn func(port string, https bool)) *ServerHandler
func (h *ServerHandler) SetStore(s Store) *ServerHandler
func (h *ServerHandler) SetUI(ui UI) *ServerHandler
func (h *ServerHandler) SetOnExternalModeExecution(fn func(bool)) *ServerHandler
func (h *ServerHandler) SetGitIgnoreAdd(fn func(string) error) *ServerHandler
func (h *ServerHandler) SetCompileArgs(fn func() []string) *ServerHandler
func (h *ServerHandler) SetRunArgs(fn func() []string) *ServerHandler
func (h *ServerHandler) SetDisableGlobalCleanup(disable bool) *ServerHandler
```

### `RegisterRoutes` — route registration pattern

```go
// RegisterRoutes appends fn to the internal route list.
// Same pattern as assetmin.AssetMin.RegisterRoutes.
// Call before StartServer.
func (h *ServerHandler) RegisterRoutes(fn func(*http.ServeMux)) *ServerHandler
```

This replaces `Config.Routes []func(*http.ServeMux)` entirely.

### Usage examples

```go
// Minimal (test):
srv := server.New()
srv.RegisterRoutes(myHandler.RegisterRoutes)
srv.StartServer(nil)

// Full (production from main.go):
srv := server.New().
    SetAppRootDir(startDir).
    SetLogger(logger.Logger).
    SetExitChan(exitChan).
    SetStore(db).
    SetUI(ui).
    SetOpenBrowser(browser.OpenBrowser).
    SetGitIgnoreAdd(gitHandler.GitIgnoreAdd).
    SetCompileArgs(func() []string { return []string{"-p", "1"} }).
    SetRunArgs(func() []string { ... })
// Routes registered by InitBuildHandlers:
srv.RegisterRoutes(assets.RegisterRoutes)
srv.RegisterRoutes(client.RegisterRoutes)
```

---

## ServerInterface Conformance

`ServerHandler` already satisfies the interface — only the assertion is new.

| Method | Present? | Location |
|---|---|---|
| `StartServer(wg *sync.WaitGroup)` | ✅ | `management.go` |
| `StopServer() error` | ✅ | `management.go` |
| `RestartServer() error` | ✅ | `management.go` |
| `NewFileEvent(fileName, extension, filePath, event string) error` | ✅ | `NewFileEvent.go` |
| `UnobservedFiles() []string` | ✅ | `server.go` |
| `SupportedExtensions() []string` | ✅ | `server.go` |
| `Name() string` | ✅ | `tui.go` |
| `Label() string` | ✅ | `tui.go` |
| `Value() string` | ✅ | `tui.go` |
| `Change(v string) error` | ✅ | `tui.go` |
| `RefreshUI()` | ✅ | `tui.go` |

### `server/server.go` — compile-time assertion

```go
// NOTE: circular dep prevents importing app — mirror the interface locally.
var _ serverInterface = (*ServerHandler)(nil)

type serverInterface interface {
    StartServer(wg *sync.WaitGroup)
    StopServer() error
    RestartServer() error
    NewFileEvent(fileName, extension, filePath, event string) error
    UnobservedFiles() []string
    SupportedExtensions() []string
    Name() string
    Label() string
    Value() string
    Change(v string) error
    RefreshUI()
}
```

---

## Methods NOT in the common interface (server-specific)

Accessed via type assertion in `section-build.go` if needed:

| Method | Purpose |
|---|---|
| `SetOnExternalModeExecution(fn func(bool))` | Wired after routes are registered |
| `SetExternalServerMode(external bool) error` | Switch Internal ↔ External strategy |
| `CreateTemplateServer() error` | Generate external server template file |
| `MainInputFileRelativePath() string` | Path helper for external strategy |

---

## Modified Files Summary

| File | Change |
|---|---|
| `server/server.go` | `New()` zero-arg, add all Set* methods, add `RegisterRoutes`, add compile-time assertion |
| `server/docs/SERVER_INTERFACE.md` | This document |

## Verification

```bash
cd tinywasm/server && gotest
# Compile-time: if SetHandler drifts from interface → build error immediately.
# Functional: existing tests must pass unchanged.
```
