# Default GoServer Implementation

This is the default server implementation that gets created when no external server exists.
You can modify this server according to your needs.

## Server Code

```go
//go:build !wasm

package main

import (
	"github.com/tinywasm/env"
	"github.com/tinywasm/server/httpd"
)

const (
	argPort      = "server_port"
	argPublicDir = "server_public_dir"
	argNoCache   = "server_no_cache"
)

func main() {
	port := env.Arg(argPort)
	if port == "" {
		port = "{{.AppPort}}"
	}
	publicDir := env.Arg(argPublicDir)
	if publicDir == "" {
		publicDir = "{{.PublicDir}}"
	}

	httpd.New(httpd.Config{
		Port:      port,
		PublicDir: publicDir,
		Gzip:      true,
		NoCache:   env.Arg(argNoCache) == "true", // default false: cache enabled
		Health:    true,
	}).ListenAndServe()
}
```
