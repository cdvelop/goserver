# Plan — `server/httpd`: identidad global y `Handler()` testeable

> Autocontenido, en español. Rige el **arnés de construcción** (reglas en el
> `AGENTS.md` de la raíz de esta librería).
> Alcance: **solo el subpaquete `httpd/`** (adaptador nativo de `router` con baterías).
> Depende de [`router/docs/PLAN.md`](../../router/docs/PLAN.md) (`Context.UserID()`).

---

## Problema 1 — la identidad solo corre en rutas con `.Requires`

En [`httpd/adapter.go`](../httpd/adapter.go), `Handle` solo invoca `identify(ctx)`
cuando `route.info.Resource != ""` (RBAC de ruta). Los módulos montados que hacen su
**propio** RBAC interno — p. ej. `mcp` en `POST /mcp`, que no declara `.Requires`
porque decide por-tool — **nunca reciben la identidad**: su handler recibe un `Context`
sin `UserID()` seteado. Es el mismo agujero silencioso que documenta
[`mcp/docs/PLAN.md`](../../mcp/docs/PLAN.md).

### Corrección — middleware de autenticación **global**

El host registra un `router.Middleware` de identidad que corre para **todas** las
peticiones, antes de cualquier handler (incluidos los módulos montados). `httpd`
expone la costura tipada:

```go
type Config struct {
	// ...Port, PublicDir, Gzip, NoCache, Health...

	// Authn corre para toda petición y deja la identidad en el Context
	// (ctx.SetUserID). Lo provee tinywasm/user (Authenticate()). nil = anónimo.
	Authn router.Middleware

	// Authorize decide el RBAC de ruta (resource, action string). Requerido si
	// alguna ruta declara .Requires(...). Reutiliza el tipo del contrato.
	Authorize router.Authorize

	RoutesEndpoint bool
	TLS            TLSConfig
}
```

- `Config.Identify func(router.Context) string` **se elimina**: con
  `router.Context.UserID()` la identidad ya es tipada y accesible; el RBAC de ruta lee
  `ctx.UserID()` directamente. Una sola forma (arnés), no dos fuentes de identidad.
- `ListenAndServe` aplica `Authn` como middleware global (vía el `useGlobal` ya
  existente) antes de resolver rutas y estáticos.

---

## Problema 2 — no hay `http.Handler` para `httptest`

`ListenAndServe` construye el handler y abre el puerto en un solo paso. Un consumidor
(p. ej. `mjosefa-cms/tests/`) no puede ejercitar el servidor con `httptest` sin abrir
un puerto real. La maquinaria (`wrapWithBatteries`, `mux`) es privada — correcto, pero
falta la **costura de test** pública.

### Corrección — exponer el handler ya cableado

```go
// Handler devuelve el http.Handler completo (estáticos + baterías + rutas +
// RBAC + Authn), sin abrir puerto. Aplica las mismas validaciones que
// ListenAndServe (RBAC, TLS-config) y por eso puede fallar.
func (s *Server) Handler() (http.Handler, error)
```

`ListenAndServe` pasa a construir su handler llamando a `Handler()` (una sola ruta de
armado; sin lógica duplicada). Los tests hacen:

```go
h, err := srv.Handler()
req := httptest.NewRequest("POST", "/mcp", body)
w := httptest.NewRecorder()
h.ServeHTTP(w, req)
```

---

## Cambios

| Archivo | Cambio |
|---|---|
| `httpd/httpd.go` | `Config`: quitar `Identify`, añadir `Authn router.Middleware`; retipar `Authorizer` → `Authorize router.Authorize`. Extraer `buildHandler()` y añadir `Handler() (http.Handler, error)`. |
| `httpd/adapter.go` | RBAC de ruta lee `ctx.UserID()` (no `identify`). Aplicar `Authn` global. Sin `Public()` ni permiso ⇒ denegar. |
| `httpd/enforce.go` | `validateRBAC` referencia `Config.Authorize`; se ejecuta desde `Handler()` (fail-fast). |
| `httpd/*_test.go` | Cubrir: identidad global llega a un módulo montado sin `.Requires`; `Handler()` sirve `httptest`. |

---

## Estrategia de pruebas y criterios de aceptación

- **Identidad global:** un `Authn` de prueba que hace `ctx.SetUserID("u1")`; un módulo
  montado sin `.Requires` lee `ctx.UserID() == "u1"` en su handler. Hoy leería `""`.
- **`Handler()` testeable:** `httptest` ejercita `/health`, estáticos+no-cache, gzip,
  montaje de `router.APIModule`, y RBAC de ruta (403/allow) sin abrir puerto.
- **Una sola ruta de armado:** `ListenAndServe` usa `Handler()` internamente; no hay
  construcción de handler duplicada.
- Ninguna firma exportada nueva nombra tipos `http.*` salvo el `http.Handler` de
  retorno de `Handler()` (borde de I/O admitido por el arnés).

---

## Endurecimiento de seguridad (cerrado por defecto) — cada punto con test

`httpd` es el aplicador del RBAC de ruta; su mecanismo debe cerrar por defecto.

- **Identidad por campo tipado, no por "recordar".** La identidad se cablea vía
  `Config.Authn router.Middleware` (corre global), nunca una llamada `Use(...)`
  imperativa que se pueda omitir. Ausente ⇒ toda petición es anónima (y el RBAC de ruta
  la deniega salvo `Public()`).
  **Test:** con `Config{Authn: nil}` y una ruta `.Requires("x","r")`, una petición sin
  identidad recibe `403` (no pasa por defecto).
- **Fail-fast al construir, no al servir.** `Handler()` (y por tanto `ListenAndServe`)
  ejecuta `validateRBAC`: si una ruta declara `.Requires(...)` y falta `Authorize`,
  devuelve **error de construcción**, no un arranque inseguro.
  **Test:** `New(Config{}).Mount(<módulo con ruta .Requires>)` → `Handler()` devuelve
  error; con `Authorize` presente → `Handler()` ok.
- **`Public()` es explícito.** Una ruta sin `Public()` ni permiso concedido se **deniega**
  al anónimo; `Public()` (del contrato `router`) es el único camino a acceso sin auth.
  **Test:** ruta `Public()` → anónimo `200`; misma ruta sin `Public()` → anónimo `403`.

---


actueliza la docuemntacion y test
