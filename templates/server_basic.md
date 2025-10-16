# Default GoServer Implementation

This is the default server implementation that gets created when no external server exists.
You can modify this server according to your needs.

## Server Code

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "{{.AppPort}}"
    }

    // Default to "public" but allow overriding with PUBLIC_DIR env var
    publicDir := os.Getenv("PUBLIC_DIR")
    if publicDir == "" {
        publicDir = "public"
    }

    // Serve static files
    fs := http.FileServer(http.Dir(publicDir))

    // Use a dedicated ServeMux so we can pass it to an http.Server
    mux := http.NewServeMux()
    mux.Handle("/", fs)

    // Health check endpoint
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, "Server is running")
    })

    // Create http.Server with Addr and Handler set
    server := &http.Server{
        Addr:    ":" + port,
        Handler: mux,
    }

    fmt.Printf("Server starting on port %s — Serving static files from: %s\n", port, publicDir)

    if err := server.ListenAndServe(); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}
```
