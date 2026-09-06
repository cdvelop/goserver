# Default GoServer Implementation

This is the default server implementation that gets created when no external server exists.
You can modify this server according to your needs.

## Server Code

```go
//go:build !wasm

package main

import (
	"log"

	"webtyp.com/env"
	"webtyp.com/server/httpd"
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

	s := httpd.New(httpd.Config{
		Port:      port,
		PublicDir: publicDir,
		Gzip:      true,
		NoCache:   env.Arg(argNoCache) == "true", // default false: cache enabled
		Health:    true,
	})

	// This process runs standalone (External mode), so routes registered via
	// ServerHandler.RegisterRoutes in Internal mode do NOT carry over here —
	// those are Go closures in the dev process and can't cross the process
	// boundary. Add your real routes/API modules directly below, e.g.:
	//   s.Router().Get("/api/hello", func(ctx router.Context) { ... })
	//   s.Mount(myAPIModule) // webtyp.com/router.APIModule
	//
	// This file is only generated once (never overwritten) — edit it freely.

	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
```
