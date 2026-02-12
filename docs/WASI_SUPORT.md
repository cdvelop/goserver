# Dynamic WASM Server with Hot-Reload

## Architecture

```
web/
 ├── server.go
 ├── modules/
 │    ├── users.wasm
 │    └── auth.wasm
 │
 └── middleware/
      ├── cors@[*].wasm
      ├── jwt@[users,auth].wasm
      └── log@[-auth].wasm
```

All requests pass through a dynamic dispatcher without restarting the server.

## Dependencies

```bash
go get github.com/fsnotify/fsnotify
go get github.com/tetratelabs/wazero
```

## Implementation - server.go

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/tetratelabs/wazero"
)

type Module struct {
	handle func() string
}

var (
	modules = map[string]*Module{}
	mu      sync.RWMutex
)

func loadWasm(rt wazero.Runtime, path string) *Module {

	ctx := context.Background()

	bin, err := os.ReadFile(path)
	if err != nil {
		log.Println(err)
		return nil
	}

	mod, err := rt.Instantiate(ctx, bin)
	if err != nil {
		log.Println(err)
		return nil
	}

	fn := mod.ExportedFunction("handle")

	return &Module{
		handle: func() string {
			res, _ := fn.Call(ctx)
			ptr := uint32(res[0])
			mem := mod.Memory().Buffer()
			return string(mem[ptr : ptr+128])
		},
	}
}

func watch(rt wazero.Runtime, dir string) {

	watcher, _ := fsnotify.NewWatcher()

	watcher.Add(dir)

	go func() {
		for e := range watcher.Events {

			if !strings.HasSuffix(e.Name, ".wasm") {
				continue
			}

			name := strings.TrimSuffix(filepath.Base(e.Name), ".wasm")

			log.Println("loading", name)

			m := loadWasm(rt, e.Name)
			if m == nil {
				continue
			}

			mu.Lock()
			modules[name] = m
			mu.Unlock()
		}
	}()
}

func main() {

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	// preload existing
	files, _ := filepath.Glob("web/modules/*.wasm")
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".wasm")
		modules[name] = loadWasm(rt, f)
	}

	watch(rt, "web/modules")

	http.HandleFunc("/m/", func(w http.ResponseWriter, r *http.Request) {

		name := strings.TrimPrefix(r.URL.Path, "/m/")

		mu.RLock()
		mod := modules[name]
		mu.RUnlock()

		if mod == nil {
			http.NotFound(w, r)
			return
		}

		w.Write([]byte(mod.handle()))
	})

	log.Println("listening :8080")
	http.ListenAndServe(":8080", nil)
}
```

## Middleware Rule System

Filename syntax:
```
name@[rules].wasm
```

| Rule | Example | Behavior |
|------|---------|----------|
| All | `cors@[*].wasm` | Applies to all modules |
| Specific | `jwt@[users,auth].wasm` | Applies only to `users` and `auth` |
| Except | `rate@[-auth].wasm` | Applies to all except `auth` |

## Rule Parser

```go
type Rule struct {
	All    bool
	Only   []string
	Except []string
}

func parseRule(filename string) Rule {
	start := strings.Index(filename, "@[")
	if start == -1 {
		return Rule{All: true}
	}

	end := strings.Index(filename, "]")
	raw := filename[start+2 : end]

	if raw == "*" {
		return Rule{All: true}
	}

	var r Rule
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "-") {
			r.Except = append(r.Except, s[1:])
		} else {
			r.Only = append(r.Only, s)
		}
	}
	return r
}
```

## Dynamic Pipeline

```go
func pipeline(route string) []Middleware {
	var out []Middleware

	for _, mw := range middlewares {
		if mw.Rule.All {
			out = append(out, mw)
			continue
		}

		if slices.Contains(mw.Rule.Only, route) {
			out = append(out, mw)
		}

		if slices.Contains(mw.Rule.Except, route) {
			continue
		}
	}
	return out
}
```

## Request Flow

```
HTTP → Go Dispatcher → Dynamic Middlewares → WASM Module
```

## Compatibility

- **fsnotify**: Linux (inotify), Windows, macOS, BSD
- **wazero**: Pure Go → Cross-platform (Linux, Windows, macOS, ARM, x86)

## Production Considerations

- WASM checksums
- Context timeouts
- Panic recovery
- Versioning
- Unload obsolete modules
