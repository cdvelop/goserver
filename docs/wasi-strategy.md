# Base 1: tinywasm/server — WASI Strategy

## New Files

### `server/wasi_module.go`
Module lifecycle management using wazero.

```go
//go:build !wasm

package server

import (
    "context"
    "sync/atomic"
    "github.com/tetratelabs/wazero"
    "github.com/tetratelabs/wazero/api"
)

// WasiModule represents a loaded WASM module instance.
type WasiModule struct {
    name     string
    runtime  wazero.Runtime
    mod      api.Module
    active   atomic.Int32  // active request count for drain
    drainFn  api.Function  // exported drain() uint32
    initFn   api.Function  // exported init()
}

// Load compiles and instantiates a .wasm binary.
func loadWasiModule(ctx context.Context, name string, wasmBytes []byte, host wazero.HostModuleBuilder) (*WasiModule, error)

// Drain asks the module if it's ready to be replaced.
// Returns ms to wait (0 = ready). Polls until 0 or drainTimeout.
func (m *WasiModule) Drain(ctx context.Context, timeout time.Duration) error

// Close releases wazero resources.
func (m *WasiModule) Close(ctx context.Context) error
```

### `server/wasi_host.go`
Host functions exposed to WASM modules.

```go
//go:build !wasm

package server

// Host functions registered in wazero for modules to call:
//   publish(topic_ptr, topic_len, payload_ptr, payload_len)
//   subscribe(topic_ptr, topic_len, handler_fn_idx)
//   ws_broadcast(topic_ptr, topic_len, payload_ptr, payload_len)
//   log(msg_ptr, msg_len)
//
// Host functions registered for server to call on module:
//   drain() uint32       → ms to wait, 0 = ready
//   init()               → called after load, before first request
//   on_message(payload_ptr, payload_len)  → receive subscribed messages
```

### `server/wasi_strategy.go`
Third execution strategy implementing `ServerStrategy`.

```go
//go:build !wasm

package server

type wasiStrategy struct {
    cfg        *Config
    modules    map[string]*WasiModule  // name → instance
    mu         sync.RWMutex
    bus        bus.Bus                 // pub/sub hub (tinywasm/bus)
    wsHub      *wsHub                  // WebSocket relay hub
    watcher    *fsnotify.Watcher       // watches compiled .wasm output dir
    drainTimeout time.Duration         // default 5s
}

// Implements ServerStrategy:
func (s *wasiStrategy) Start(wg *sync.WaitGroup) error
func (s *wasiStrategy) Stop() error
func (s *wasiStrategy) Restart() error
func (s *wasiStrategy) HandleFileEvent(fileName, extension, filePath, event string) error
func (s *wasiStrategy) Name() string  // returns "wasi"

// Hot-swap a single module (triggered when .wasm file changes):
func (s *wasiStrategy) swapModule(ctx context.Context, name string, wasmBytes []byte) error
```

### `server/ws_hub.go`
WebSocket relay hub (modules publish → clients receive).

```go
//go:build !wasm

package server

// wsHub manages all WebSocket connections.
// Clients subscribe to topics; modules publish via host functions.
// Endpoint registered on mux: GET /ws?topic=TOPIC_NAME
type wsHub struct {
    clients map[string]map[*wsConn]bool  // topic → set of connections
    mu      sync.RWMutex
    bus     bus.Bus
}

func (h *wsHub) RegisterRoute(mux *http.ServeMux)
func (h *wsHub) Broadcast(topic string, msg []byte)
```

## Modified Files

### `server/strategies.go`
Add `wasiStrategy` to the strategy selection logic (new case in switch).

### `server/server.go` — Config changes
```go
type Config struct {
    // ... existing fields ...

    // NEW: WASI module system config
    WasiModulesDir  string          // e.g., "modules" (relative to AppRootDir)
    WasiOutputDir   string          // where compiled .wasm files are placed
    WasiDrainTimeout time.Duration  // default 5s; 0 = use default
    WasiBus         bus.Bus         // optional: inject external bus instance
}
```

### `server/templates/server_basic.md`
Add optional WASI module loading block at the end of the template.

## New Dependency
```bash
go get github.com/tetratelabs/wazero@latest
```
