# Default GoServer Implementation

This is the default server implementation that gets created when no external server exists.
You can modify this server according to your needs.

## Server Code

```go
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// Define flags
	publicDir := flag.String("public-dir", "", "Directory containing static files")
	port := flag.String("port", "", "Port to listen on")
	flag.Parse()

	// Priority: flag > env var > default
	if *port == "" {
		*port = os.Getenv("PORT")
		if *port == "" {
			*port = "{{.AppPort}}"
		}
	}

	if *publicDir == "" {
		*publicDir = os.Getenv("PUBLIC_DIR")
		if *publicDir == "" {
			*publicDir = "public"
		}
	}

	// Make it absolute if it's relative
	absPublicDir, err := filepath.Abs(*publicDir)
	if err != nil {
		log.Fatalf("Error resolving public directory path: %v", err)
	}

	// Verify the directory exists
	if _, err := os.Stat(absPublicDir); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", absPublicDir)
	}

	log.Printf("Serving static files from: %s", absPublicDir)
	fs := http.FileServer(http.Dir(absPublicDir))

	// Middleware to disable caching for static files (useful in dev/test)
	noCache := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			h.ServeHTTP(w, r)
		})
	}

	mux := http.NewServeMux()
	mux.Handle("/", noCache(fs))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Server is running"))
	})

	server := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
	}

	log.Printf("Starting server on port %s", *port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
```
