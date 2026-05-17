# Server Interface & API

Common contract and configuration for `tinywasm/server`. This package provides a high-performance HTTP server handler designed to work with both embedded and external execution strategies.

## API

### `server.New()` — Constructor

Creates a new `ServerHandler` with zero arguments. All configuration is optional and handled via method chaining.

```go
srv := server.New()
```

### Default Configuration

| Field | Default Value | Description |
|---|---|---|
| `AppRootDir` | `"."` | Application root directory |
| `SourceDir` | `"web"` | Directory of `main.go` (relative to `AppRootDir`) |
| `OutputDir` | `"web"` | Compilation and execution directory |
| `PublicDir` | `"web/public"` | Public assets directory |
| `MainInputFile` | `"main.go"` | Main entry point file name |
| `AppPort` | `"6060"` | Server port |
| `Https` | `false` | Enable/Disable HTTPS |
| `ExitChan` | `make(chan bool)` | Channel to signal shutdown |

### Configuration Methods (Chaining)

All methods return `*ServerHandler` to allow for fluent API usage.

```go
func (h *ServerHandler) SetAppRootDir(dir string) *ServerHandler
func (h *ServerHandler) SetSourceDir(dir string) *ServerHandler
func (h *ServerHandler) SetOutputDir(dir string) *ServerHandler
func (h *ServerHandler) SetPublicDir(dir string) *ServerHandler
func (h *ServerHandler) SetMainInputFile(name string) *ServerHandler
func (h *ServerHandler) SetPort(port string) *ServerHandler
func (h *ServerHandler) SetHTTPS(enabled bool) *ServerHandler
func (h *ServerHandler) SetLogger(fn func(...any)) *ServerHandler
func (h *ServerHandler) SetExitChan(ch chan bool) *ServerHandler
func (h *ServerHandler) SetOpenBrowser(fn func(port string, https bool)) *ServerHandler
func (h *ServerHandler) SetStore(s Store) *ServerHandler
func (h *ServerHandler) SetUI(ui UI) *ServerHandler
func (h *ServerHandler) SetGitIgnoreAdd(fn func(string) error) *ServerHandler
func (h *ServerHandler) SetCompileArgs(fn func() []string) *ServerHandler
func (h *ServerHandler) SetRunArgs(fn func() []string) *ServerHandler
func (h *ServerHandler) SetDisableGlobalCleanup(disable bool) *ServerHandler
```

### Route Registration

Register application routes before starting the server.

```go
// RegisterRoutes appends a registration function to the internal list.
func (h *ServerHandler) RegisterRoutes(fn func(*http.ServeMux)) *ServerHandler
```

**Example:**
```go
srv := server.New().
    SetPort("8080").
    RegisterRoutes(api.RegisterRoutes).
    RegisterRoutes(web.RegisterRoutes)
```

---

## ServerInterface Conformance

`ServerHandler` satisfies the common `ServerInterface` used by `tinywasm/app`.

| Method | Description |
|---|---|
| `StartServer(wg *sync.WaitGroup)` | Starts the server lifecycle |
| `StopServer() error` | Stops the server |
| `RestartServer() error` | Restarts the server strategy |
| `NewFileEvent(...) error` | Handles file change events |
| `UnobservedFiles() []string` | Returns files ignored by watchers |
| `SupportedExtensions() []string` | Returns handled file extensions (`.go`) |
| `Name()`, `Label()`, `Value()` | TUI Metadata |
| `Change(v string) error` | TUI Mode toggle |
| `RefreshUI()` | Triggers UI update |

### Compile-time Assertion

```go
var _ serverInterface = (*ServerHandler)(nil)
```

---

## External Server Mode

Methods specific to the external execution strategy:

| Method | Purpose |
|---|---|
| `SetBeforeExternalServerStart(fn func() error)` | Hook invoked before external process start. Errors abort startup. |
| `SetExternalServerMode(external bool) error` | Explicitly switch strategy |
| `CreateTemplateServer() error` | Generate and run external server files |
| `MainInputFileRelativePath() string` | Resolution helper |

### BeforeExternalServerStart Hook

The hook registered via `SetBeforeExternalServerStart` is invoked synchronously BEFORE `strategy.Start` in every external-mode `StartServer` call.

- **Errors:** Returning a non-nil error aborts the transition; `strategy.Start` is not invoked and the error is logged.
- **Idempotency:** The hook fires on every external-mode `StartServer`, not just on transitions.
- **Restarts:** `RestartServer` does NOT invoke this hook.

---

## Verification

Run tests to ensure API stability:
```bash
cd tinywasm/server && gotest
```
