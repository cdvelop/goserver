# Plan — Refactor de `server` al contrato de enrutado `tinywasm/router`

> Reemplaza al plan anterior ("Fix publicDir — server_public_dir"), **ya ejecutado**
> (el template `server_basic.md` usa `lookupArg("server_public_dir")` y `app` pasa el
> argumento). Este PLAN aborda la migración del servidor al contrato isomórfico
> `github.com/tinywasm/router`. Autocontenido, en español.
>
> Nota: este archivo fue movido desde `PLAN.md` a `PLAN_ROUTER_REFACTOR.md` para dejar
> sitio a `docs/PLAN.md` como orquestador junto con `PLAN_HOTRELOAD.md`. Ver
> `docs/PLAN.md` para el orden de despacho. Confirmado por inspección de
> `strategies.go`/`server.go` que este refactor **aún no se ha ejecutado** (siguen
> importando `net/http`, no hay dependencia a `tinywasm/router` en `go.mod`).

---

## Reglas de Desarrollo

Las reglas del arnés viven en el **`AGENTS.md` de la raíz de esta librería** — léelo
antes de cualquier cambio. Este PLAN no las repite; describe solo el *cómo*.

Alcance (responsabilidad única): orquestar el arranque/parada de un servidor local y
sus estrategias. **No** debe definir su propia abstracción de rutas ni exponer tipos
de `net/http` en su superficie pública.

---

## El contrato que consume (reexpresado para ser autocontenido)

```go
// package router (github.com/tinywasm/router)
type Context interface { Method() string; Path() string; Body() []byte
	GetHeader(k string) string; SetHeader(k, v string); WriteStatus(code int); Write([]byte) (int, error) }
type HandlerFunc func(Context)
type Router interface {
	Get(path string, h HandlerFunc); Post(path string, h HandlerFunc)
	Put(path string, h HandlerFunc); Delete(path string, h HandlerFunc)
	Options(path string, h HandlerFunc); Handle(method, path string, h HandlerFunc)
	Stream(path string, h StreamFunc); Socket(path string, h SocketFunc); Use(m ...Middleware)
}
```

---

## Estado de partida

- `type ServerStrategy interface { … }` con estrategias `internalStrategy` /
  `externalStrategy`.
- `internalStrategy` construye `http.NewServeMux()` y registra con
  `mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){ … })`; sirve con
  `http.Server`.
- La superficie de registro de rutas expone `*http.ServeMux` al exterior.

---

## Cambios (antes → después)

| Antes (`net/http`) | Después (`router`) |
|---|---|
| `RegisterRoutes(fn func(*http.ServeMux))` | `RegisterRoutes(fn func(router.Router))` |
| `mux.HandleFunc(path, func(w,r))` | `r.Handle(method, path, func(router.Context))` |
| `internalStrategy` construye y sirve un `*http.ServeMux` | sirve un **implementador** de `router.Router` (reutiliza el implementador nativo del ecosistema; no duplica uno) |

`server` deja de exponer `*http.ServeMux`. El `http.ListenAndServe`/`http.Server`
interno queda **encapsulado** detrás del implementador de `router.Router`; no aparece
en la API.

---

## Pasos de implementación

1. Añadir dependencia `github.com/tinywasm/router` en `go.mod`.
2. Cambiar las firmas públicas de registro de rutas de `*http.ServeMux` a
   `router.Router`.
3. Hacer que la estrategia interna sirva sobre un implementador nativo de
   `router.Router` (reutilizado del ecosistema), no sobre un `ServeMux` propio.
4. Adaptar los handlers internos a `router.HandlerFunc(router.Context)`.

---

## Estrategia de pruebas y criterios de aceptación

- **Sin `net/http` en la superficie pública:** ninguna firma exportada nombra
  `http.ServeMux`/`http.Handler`/`http.ResponseWriter`. Verificable por búsqueda.
- **Arranque/parada intactos:** los tests de ciclo de vida (start/stop/restart)
  siguen pasando sobre el nuevo implementador.
- **Registro por contrato:** un test registra una ruta vía `func(router.Router)` y
  la sirve; el handler recibe un `router.Context`, no `w,r`.
