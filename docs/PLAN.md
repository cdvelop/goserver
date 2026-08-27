---
PLAN: "feat: path parameters in httpd, and one shared introspection endpoint"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 7235600109958421322
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — path parameters in `httpd`, and stop owning the introspection endpoint

## Why this exists

Two things, both about the same asymmetry between this repo and the edge
runtime.

**1. `httpContext` has no `Param`.** `httpd` registers every route straight into
a `net/http.ServeMux` as `"GET /path"`, and ServeMux has matched `{id}` patterns
natively since Go 1.22 — so this repo can already *match* a parameterised route.
It just has no way to hand the value to a handler, so nobody uses one, and every
app in the ecosystem dispatches by path suffix inside the handler instead.

**2. This repo owns `/_routes` alone.** `httpd/routes_endpoint.go` is the only
place in the ecosystem that serves the route table, so the deployed edge runtime
— the thing you most need to interrogate — cannot answer the question at all.
That endpoint has now moved up into `tinywasm/router` as `MountIntrospection`,
where every implementation shares one copy. This repo consumes it instead of
having its own.

The endpoint also gains something it could not report before: **which roles hold
the permission a route requires.** In `veltylabs/misitio`, six of eleven
routes require a `(Resource, Action)` pair granted to no role at all — all eight
answer `403` to everybody while looking perfectly declared. `httpd.Config` gains
a `Policy` field so `/_routes` can surface that.

## Dependency — read before you start

This plan requires **`github.com/tinywasm/router`** at the version exporting
`Context.Param`, `ValidatePattern`, `IntrospectionPath` and
`MountIntrospection`, and **`github.com/tinywasm/model`** at the version
exporting `PolicyDescriber`. Both are **already published** when this plan is
dispatched.

```
go get github.com/tinywasm/router@latest
go get github.com/tinywasm/model@latest
go mod tidy
```

**Never** add a `replace` directive, never invent a version, and never
re-declare the upstream symbols locally. If `go get` does not yield a `router`
with `MountIntrospection`, stop and report it.

`Param` is a new method on `router.Context`: `*httpContext` stops compiling
until Stage 1 is done. That is expected and coordinated across the wave.

## Anti-footguns

- **This repo is backend tooling.** It legitimately imports `net/http`, `sync`,
  `io`, `time` and the rest of the standard library. The ecosystem's
  "no stdlib" rule applies to WASM-compiled packages — do **not** "fix" the
  stdlib imports here.
- **Do not reimplement matching.** ServeMux does it. The one thing this repo
  adds is `router.ValidatePattern` at registration, so a pattern the *edge*
  runtime rejects cannot be silently accepted here — that divergence is exactly
  what the wave exists to close.
- Keep `Config.RoutesEndpoint`. It is a published field with live consumers
  (`veltylabs/misitio`'s `web/server.go`, this repo's `batteries_test.go` and
  `concurrency_test.go`). Its **implementation** changes; its name and meaning
  do not.

---

## Stage 1 — `httpContext.Param`

File: **`httpd/adapter.go`**.

```go
// Param returns a path parameter the matched route declared with {name}.
//
// ServeMux extracts it: the route was registered as "GET /api/items/{id}",
// which the standard library has matched and populated since Go 1.22. This
// repo does not parse patterns — see docs/ARCHITECTURE.md.
func (c *httpContext) Param(name string) string {
	return c.r.PathValue(name)
}
```

That is the whole implementation. Do not add a parameter store to
`httpContext`, and do not copy the value into `values` — `Value` is what a
middleware wrote and `Param` is what the URL carried; collapsing them lets a
middleware forge a path parameter.

## Stage 2 — Validate patterns at registration

File: **`httpd/adapter.go`**, in `(*httpRouter).Handle` — the funnel every
registration method goes through.

```go
if err := router.ValidatePattern(path); err != nil {
	panic(err.Error())
}
```

A panic, because registration happens at startup and `Handle` returns a
`router.Route` with nowhere to put an error — the same reasoning that already
makes `ServeMux` panic on a duplicate pattern.

This is the one place this repo is **stricter than ServeMux on purpose**:
ServeMux honours `{name...}` trailing wildcards and the edge runtime does not,
so `router.ValidatePattern` rejects them for everyone. A pattern that works on
the dev server and 404s in the Worker is the worst bug this contract can ship.
State that reasoning in a comment next to the call.

Apply it in `PublicAsset` and `PublicDir` too.

## Stage 3 — Consume `router.MountIntrospection`

File: **`httpd/routes_endpoint.go`** — rewritten, not extended.

**Delete** `routesResponse` and its `EncodeFields`, and the direct
`s.mux.HandleFunc(...)` registration. Both now live in `tinywasm/router`. The
long comment on `routesResponse` records a real bug — reflection encoding
`Access: 0`, so the most protected route in the server reported itself as
"nothing declared" — and that reasoning **already travelled upstream with the
code**; do not duplicate it back here.

What remains:

```go
// RoutesPath is where this server exposes its route table. It is
// router.IntrospectionPath: the path is the ecosystem's, not this
// implementation's, so an operator asks the same question of a dev server and
// of a deployed Worker.
const RoutesPath = router.IntrospectionPath

func (s *Server) registerRoutesEndpoint() {
	if !s.config.RoutesEndpoint {
		return
	}
	router.MountIntrospection(s.router, RoutesPath, s.config.Policy).Public()
}
```

Three consequences to be deliberate about, all of them improvements — say so in
the code comments:

1. The endpoint is now a **real route**, registered through the router instead
   of straight onto the mux. It therefore appears in its own listing. An
   introspection endpoint that hides itself is lying by omission.
2. It is declared `.Public()`, which preserves exactly today's behaviour (the
   mux registration was reachable with no identity). It must be **explicit**:
   the zero value of `model.Access` is `AccessGuarded`, and `validateRBAC`
   refuses to start on a guarded route with no resource.
3. `registerRoutesEndpoint` is called from `Handler()`, which rebuilds the mux
   and re-registers every route. Verify the endpoint is not registered **twice**
   when `Handler()` is called more than once — if it is, guard it with a flag on
   `Server` and add a test that calls `Handler()` twice and asserts a single
   `/_routes` entry in `Routes()`.

## Stage 4 — `Config.Policy`

File: **`httpd/httpd.go`**.

```go
type Config struct {
	// ...
	Authn          router.Middleware
	Authorize      model.Authorizer
	RoutesEndpoint bool

	// Policy lets the introspection endpoint report WHICH roles hold the
	// permission each route requires. Optional: nil leaves those columns
	// marked unknown, which is not the same as "nobody" — a permission no
	// role holds turns a correctly-declared route into a permanent 403, and
	// that finding must never be confused with "the app did not say".
	//
	// It is separate from Authorize because the two answer opposite
	// questions: Authorize answers "may this user do this?", Policy answers
	// "who may do this?". A closure cannot answer the second.
	Policy model.PolicyDescriber
}
```

`applyDefaults` does not touch it — nil is a legal, meaningful value.

## Stage 5 — Tests

Alongside the existing `httpd/*_test.go` files (this repo tests in-package).

The conformance suite in `tinywasm/router` now carries the seven parameter
clauses, and `httpd/conformance_test.go` already runs it. **Verify it passes
unchanged.** If a clause fails, the bug is in Stage 1 or 2, not in the suite.

New in **`httpd/params_test.go`**:

| Test | Assert |
|---|---|
| `TestParamFromServeMux` | `GET /api/items/{id}` handler reading `ctx.Param("id")` returns `42` for `/api/items/42` |
| `TestParamIsNotAContextValue` | same request: `ctx.Value("id")` is `""` |
| `TestRegistrationRejectsWildcard` | `r.Get("/x/{a...}", h)` panics with `router.ValidatePattern`'s message — **this repo is stricter than ServeMux on purpose** |

Extend **`httpd/routes_endpoint_test.go`**:

| Test | Assert |
|---|---|
| `TestRoutesEndpointListsItself` | with `RoutesEndpoint: true`, the response contains an entry whose path is `RoutesPath` |
| `TestRoutesEndpointReportsRoles` | with a `Policy` granting `admin → catalog:r` and a route `Requires(catalog, Read)`, that route's `roles` is `["admin"]` and `policy_known` is `true` |
| `TestRoutesEndpointReportsUnheldPermission` | policy grants nothing for the route → `roles` is `[]`, `policy_known` is `true` — the finding, distinct from unknown |
| `TestRoutesEndpointWithoutPolicy` | `Policy: nil` → `policy_known` is `false` for every route |
| `TestRoutesEndpointDisabled` | unchanged, still `404` |
| `TestRoutesEndpointRegisteredOnce` | `Handler()` called twice → exactly one `RoutesPath` entry in `Routes()` |

## Stage 6 — Documentation

- **`docs/ARCHITECTURE.md`** — a short subsection: pattern *matching* and
  *extraction* are delegated to `net/http.ServeMux`; pattern *validation* is
  delegated to `tinywasm/router` so this server cannot accept what the edge
  runtime rejects; and the introspection endpoint is `router.MountIntrospection`
  with this repo supplying only the `RoutesEndpoint` switch and the `Policy`.
- **`README.md`** — document `Config.Policy` next to `Config.Authorize`, with
  the one-sentence contrast between the two questions they answer. Add a
  `{id}` example to whatever route-registration example exists.
- Do **not** link `docs/PLAN.md` from any permanent document.

## Acceptance criteria

- [ ] `go build ./...`, `go vet ./...` clean; `gotest ./...` green.
- [ ] `grep -n "routesResponse" httpd/` → empty (moved upstream, not copied).
- [ ] `grep -n "s.mux.HandleFunc" httpd/routes_endpoint.go` → empty.
- [ ] `grep -n "RoutesPath *=" httpd/routes_endpoint.go` → the constant is
      `router.IntrospectionPath`, not a `"/_routes"` literal.
- [ ] `Config.RoutesEndpoint` still exists and still gates the endpoint;
      `batteries_test.go` and `concurrency_test.go` compile and pass unchanged.
- [ ] `httpContext.Param` is one line delegating to `r.PathValue`.
- [ ] `go.mod` has **no** `replace` directive.
- [ ] `docs/ARCHITECTURE.md` records the three delegations.

## Out of scope

Any change to `PublicDir` semantics, TLS, gzip or the handoff protocol; the
edge runtime (`tinywasm/cloudflare`, its own plan); and any HTML — this repo
serves the JSON, the explorer UI fetches it.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | `httpContext.Param` | `httpd/adapter.go` |
| 2 | Validate at registration | `httpd/adapter.go` |
| 3 | Consume `MountIntrospection` | `httpd/routes_endpoint.go` |
| 4 | `Config.Policy` | `httpd/httpd.go` |
| 5 | Tests | `httpd/params_test.go`, `httpd/routes_endpoint_test.go` |
| 6 | Documentation | `docs/ARCHITECTURE.md`, `README.md` |
