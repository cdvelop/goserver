# Architecture — `tinywasm/server/httpd`

The `httpd` subpackage provides a high-level, "batteries-included" HTTP server implementation that implements the `github.com/tinywasm/router` contract.

## Relationship with `internalStrategy`

The development mode's `internalStrategy` (in-memory server) consumes the `httpd.NewRouter` adapter to provide consistent routing behavior between development and production.

## Relationship with `templates/server_basic.md`

When a project lacks a custom `web/server.go` or switches to external mode without customization, the `generator.go` uses `templates/server_basic.md` to create a entry point. This entry point is a thin wrapper around `httpd.New()`, avoiding boilerplate duplication.

## Components

- **`adapter.go`**: Implementation of `router.Router`, `router.Context`, etc., mapping them to `net/http`.
- **`middleware.go`**: Built-in `Gzip` and `NoCache` middlewares.
- **`static.go`**: Static file serving from `PublicDir`.
- **`enforce.go`**: RBAC enforcement based on `Requires` metadata.
- **`tls.go` / `devcert.go`**: Support for AutoCert (Let's Encrypt), custom Cert/Key, and `DevTLS` (self-signed with local truststore installation).
- **`routes_endpoint.go`**: Optional JSON endpoint at `/_routes` listing all registered routes.
- **`httpd.go`**: Core `Server` orchestrator.

## Design Goals

1. **Simple Entry Point**: `httpd.New(config).Mount(modules).ListenAndServe()` is the only way to start the server.
2. **Standard Library Based**: Uses `net/http` internally but doesn't expose it in the public API.
3. **Fails Fast**: Validates configuration (like TLS modes or RBAC requirements) at startup rather than at runtime.

## RBAC and Public Assets

The server follows a **secure-by-default** model where routes are private unless explicitly marked as `Public()`. However, for a smooth development experience:

- **Static Assets**: Files served from `PublicDir` (e.g., WASM binaries, JS, CSS) are always public. They bypass the RBAC middleware.
- **Root Path (`/`) in Development**: The internal strategy registers a default public route for `/` that serves `PublicDir/index.html` or a diagnostic message. This ensures that the frontend application is accessible immediately without manual RBAC configuration.
