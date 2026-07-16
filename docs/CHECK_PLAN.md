---
PLAN: "feat: httpd implements Route.Accepts and Context.Decode/Encode"
TAG: v0.2.32
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: **agents-workflow**.
> Orquestado por `tinywasm/app-releases/docs/REUSABLE_MODULES_MASTER_PLAN.md` — **Fase B2**.

# PLAN — `server/httpd`: `Route.Accepts` + `Context.Decode`/`Encode`

Autocontenido, en español. Eres un agente **sin contexto previo** y **solo tienes este repo**
(`tinywasm/server`). Todo el contrato y el código exacto van inline.

## 1. Qué cambia y por qué

`tinywasm/router` (`v0.1.14`) amplió **dos** interfaces que `httpd` implementa:

```go
// router.Route — gana:
Accepts(args model.Fielder) Route   // + RouteInfo.Args model.Fielder

// router.Context — gana:
Decode(into model.Decodable) error
Encode(v model.Encodable) error
```

`httpd` es el implementador **nativo** de estos contratos (`httpRoute`, `httpContext` en
`httpd/adapter.go`). Al subir `go.mod` a `router@v0.1.14` **no compila**: `httpRoute` y `httpContext`
dejan de satisfacer sus interfaces (`missing method Accepts`, `missing method Decode`). Este plan
cierra exactamente eso.

> **`httpd` NO implementa `Op` ni `OpRegistry`.** El boceto anterior de este plan enseñaba a `httpd` a
> proyectar operaciones como `POST /op/{name}`. **Eso se descartó** (ver
> `REUSABLE_MODULES_MASTER_PLAN.md` §3.3c): `Op` salió de `router.Router` y pasó a `router.OpRegistry`,
> que **solo `mcp` implementa** (cosecha cada `Op` como un tool). Como el navegador y el LLM llaman las
> operaciones por `/mcp` (`mcp.NewCaller`), **no existe consumidor** para una proyección REST
> `POST /op/{name}`. `httpd` se queda como router HTTP puro: sirve `/mcp` (montado por `mcp.Server`),
> assets y auth. **No añadas `OpPrefix`, `opPath`, `httpRouter.Op` ni nada relacionado con `Op`.**

## 2. Estado actual exacto (verificado, no supuesto)

Todo vive en **un archivo**: `httpd/adapter.go`.

- `httpRouter` (adapter.go:143-153) implementa `router.Router` por
  `Get/Post/Put/Delete/Options/Handle/Stream/Socket/PublicAsset/PublicDir`. **No se toca** — con
  `router@v0.1.14`, `Router` ya **no** exige `Op`, así que `httpRouter` sigue satisfaciéndolo tal cual.
- `httpRoute` (adapter.go:155-160) implementa `router.Route`: `Requires`/`Authenticated`/`Public`
  (adapter.go:162-177), todos mutando `r.info`. Le falta `Accepts`.
- `httpContext` (adapter.go:20-27) implementa `router.Context`. Le faltan `Decode`/`Encode`. Ningún
  archivo de `httpd` importa `encoding/json`; `routes_endpoint.go` ya usa `github.com/tinywasm/json`
  (dependencia **directa** existente en `go.mod`) así:
  ```go
  // routes_endpoint.go:39 (patrón a seguir)
  json.Encode(routesResponse(s.router.Routes()), &out)
  ```
  `json.Decode(input any, data model.Decodable) error` acepta `[]byte|string|io.Reader`;
  `json.Encode(data model.Encodable, output any) error` acepta `*[]byte|*string|io.Writer`.
- `RouteInfo.Args model.Fielder` existe desde `router@v0.1.12`; **no** se serializa en
  `routes_endpoint.go` (decisión tomada aguas arriba — `router/route.go` deja `Args` fuera de
  `EncodeFields` a propósito). No lo agregues ahí.

## 3. El cambio exacto

### 3.1 `go.mod`

```
go get github.com/tinywasm/router@v0.1.14
```
Esto sube transitivamente `github.com/tinywasm/model` a `v0.0.15` (de ahí sale `model.Fielder` para
`Accepts` y `RouteInfo.Args`). **Verificado**: no hace falta ningún otro cambio de dependencia —
`github.com/tinywasm/json` ya es dependencia directa.

### 3.2 `httpRoute.Accepts`

```go
func (r *httpRoute) Accepts(args model.Fielder) router.Route {
	r.info.Args = args
	return r
}
```

Junto a `Requires`/`Authenticated`/`Public` (adapter.go:162-177).

### 3.3 `httpContext.Decode`/`Encode`

```go
func (c *httpContext) Decode(into model.Decodable) error {
	return json.Decode(c.Body(), into)
}

func (c *httpContext) Encode(v model.Encodable) error {
	var out []byte
	if err := json.Encode(v, &out); err != nil {
		return err
	}
	_, err := c.Write(out)
	return err
}
```

Añade `"github.com/tinywasm/json"` al import de `adapter.go` (ya es dependencia directa del módulo).
**No** uses `encoding/json` — ningún archivo de `httpd` lo hace hoy, y sería inconsistente con el
resto del ecosistema (el mismo codec debe correr en servidor y en WASM).

### 3.4 `conformance_test.go` — nada que conectar

El `Factory{...}` actual (`httpd/conformance_test.go`) deja `ServeOp` en `nil`, y así se queda: `httpd`
no implementa `OpRegistry`, de modo que las cláusulas `op_route_*` de `router/conformance` **se saltan
solas** (hacen `t.Skip` porque el router no satisface `OpRegistry` y/o `ServeOp` es `nil`). Eso es
correcto e intencional — no las fuerces. La cláusula `context_decodes_and_encodes_typed_payload` **sí**
corre (no tiene guarda) y debe pasar en cuanto `Decode`/`Encode` existan.

## 4. Fuera de alcance

- **No** implementes `Op`, `OpRegistry`, `OpPrefix`, `opPath`, `httpRouter.Op` ni ninguna proyección
  `POST /op/{name}` (§1). `httpd` no es transporte de operaciones — lo es `mcp`.
- **No** toques `Caller` — confirmado: ningún tipo en `tinywasm/server` implementa `router.Caller`
  (`grep -rn "router.Caller" .` vacío, excluyendo tests). El cambio de firma de `Caller.Call` en
  `router@v0.1.13` no afecta a este repo.
- **No** cambies `enforce.go`/`tls.go` a `tinywasm/fmt` — el paquete usa stdlib `fmt`/`errors`
  consistentemente hoy; no es código WASM, no aplica la regla de "cero stdlib".
- **No** serialices `RouteInfo.Args` en `routes_endpoint.go` — decisión ya tomada aguas arriba.

## 5. Criterios de aceptación

- `go build ./...` verde con `router@v0.1.14` / `model@v0.0.15`.
- `var _ router.Router = (*httpRouter)(nil)`, `var _ router.Route = (*httpRoute)(nil)`,
  `var _ router.Context = (*httpContext)(nil)` (ya existen, deben seguir compilando).
- `grep -rn "OpPrefix\|opPath\|func (r \*httpRouter) Op\b\|/op/" httpd/` **vacío** — no se introdujo
  ninguna proyección de `Op`.
- `httpd/conformance_test.go::TestHTTPDConformance` verde, con las cláusulas `op_route_*` en `SKIP`
  (esperado: `httpd` no implementa `OpRegistry`) y `context_decodes_and_encodes_typed_payload` en PASS.
- `grep -rn "encoding/json" httpd/` **vacío** (sigue usando `tinywasm/json`).
- `gotest ./...` verde (o `go test ./...` si `gotest` no está instalado en el entorno del agente).

## 6. Etapas

| # | Etapa | Archivo(s) | Criterio |
|---|---|---|---|
| 1 | Bump dependencia | `go.mod`, `go.sum` | `router@v0.1.14`, `model@v0.0.15` |
| 2 | `httpRoute.Accepts` | `adapter.go` | junto a `Requires`/`Public` |
| 3 | `httpContext.Decode`/`Encode` | `adapter.go` | vía `tinywasm/json`, sin `encoding/json` |
| 4 | Verificación | — | `go build`/`gotest` verdes; `op_route_*` en SKIP, no error |
