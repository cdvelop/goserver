# PLAN — server: `GET /` responde 403 en desarrollo; la raíz debe servir el sitio público

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Parte de `tinywasm/docs/MCP_DAEMON_HARDENING_MASTER_PLAN.md`.
> Idioma: español (decisión del mantenedor). Autocontenido: el agente no tiene contexto previo.

## Prerequisito (correr primero)

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

Todos los tests con `gotest` (nunca `go test` a secas).

---

## 0. Diagnóstico (evidencia real, 2026-07-09)

Con un proyecto mínimo servido por el server interno (`tinywasm -mcp` →
`start_development`), el dev loop visual está roto:

```
$ curl -i http://localhost:8080/
HTTP/1.1 403 Forbidden
Forbidden
```

El browser automatizado (chromedp) solo muestra "Forbidden". Un LLM no puede
ver la app que está desarrollando.

**Causa raíz (verificada en el código):**

1. `strategies.go:75` (estrategia interna, cuando `s.handler.routes` está
   vacío) registra la ruta default:

```go
r.Get("/", func(ctx router.Context) {
    ctx.SetHeader("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(ctx, "<h3>No routes registered in In-Memory Server</h3>")
})
```

   **sin marcarla `Public()`**. El RBAC de `httpd/adapter.go:270–316` es
   privado-por-defecto: ruta sin `Public()` ni `Requires()` + usuario anónimo
   (`ctx.UserID() == ""`) → `403 Forbidden` (`adapter.go:310–313`). Correcto
   como política fail-closed para rutas de negocio; incorrecto para la página
   default de desarrollo.

2. Efecto secundario: como la ruta `/` **existe**, nunca responde 404, y el
   fallback estático de `httpd/static.go` (`wrapWithBatteries`, que solo actúa
   sobre 404) **jamás sirve `PublicDir/index.html` en `/`** — ni siquiera
   cuando el archivo existe. El contenido generado por el build WASM queda
   inaccesible en la raíz.

## 1. Reglas de código (obligatorias)

- **NO debilitar el RBAC**: privado-por-defecto se mantiene para toda ruta de
  negocio. El fix es sobre la ruta default de desarrollo y el fallback
  estático, nada más.
- Strings repetidos → constantes tipadas.
- Errores se propagan; nada se traga en silencio.
- Server-side: stdlib permitida donde ya se usa.

## 2. Etapa 1 — la raíz sirve el sitio público en desarrollo

En `strategies.go` (estrategia interna, rama sin rutas registradas):

1. La ruta default `/` se registra **`Public()`** (verificar la API del router
   en `httpd/adapter.go` — `route.info.Public` se setea con el builder de la
   ruta; grep `Public()` en tests del router para el patrón exacto).
2. Su handler primero intenta servir `PublicDir/index.html` (mismo
   `s.config.PublicDir`/`DefaultPublicDir` que usa `httpd/static.go` — no
   duplicar el literal). Si existe → servirlo (con las mismas "batteries"
   NoCache/Gzip que `serveWithGlobalBatteries`). Si no existe → el mensaje
   actual "<h3>No routes registered...</h3>".
3. Revisar la otra estrategia (server generado/externo) por el mismo patrón:
   si registra una raíz default sin `Public()`, aplicar el mismo criterio.

## 3. Etapa 2 — el fallback estático debe poder servir `/`

En `httpd/static.go`, decidir e implementar UNA de estas dos (el implementador
elige la de menor invasividad y lo documenta en el código):

- (a) La ruta default de §2 ya resuelve `/`; el fallback queda como está, o
- (b) no registrar ruta default `/` y extender `wrapWithBatteries` para que el
  404 de `/` sirva `PublicDir/index.html` (ya casi lo hace: `static.go:29–34`
  contempla dir + index.html; verificar que aplique a la raíz).

Los archivos estáticos servidos por el fallback son públicos por naturaleza
(assets del frontend): confirmar que no pasan por el RBAC de rutas (hoy no
pasan — mantenerlo así y documentarlo en `docs/ARCHITECTURE.md`).

## 4. Tests (con `gotest`)

1. Server interno sin rutas + `PublicDir` con `index.html` → `GET /` anónimo
   = 200 con el contenido del index.
2. Sin `index.html` → `GET /` anónimo = 200 con el mensaje default (no 403).
3. Ruta de negocio registrada sin `Public()` → sigue 403 anónimo (regresión
   RBAC: fail-closed intacto).
4. Asset existente en `PublicDir` (p. ej. `/client.wasm`) → 200 anónimo.

## 5. Etapa 3 — `tui.go`: `ServerHandler` pasa de HandlerEdit a HandlerSelection

Hoy `ServerHandler` (en `tui.go`) es un campo de texto libre estilo
`HandlerEdit`: `Value()` devuelve `"Execution External:T"` y `Change` parsea
pares `clave:valor` a mano. El modo de ejecución es una elección **binaria y
mutuamente excluyente** — el contrato correcto es `devtui.HandlerSelection`
(radio/segmented-control), definido en `devtui/interfaces.go`:

```go
type HandlerSelection interface {
    Name() string                 // identificador para logging
    Label() string                // etiqueta del grupo de botones
    Value() string                // KEY de la opción activa
    Change(newValue string)       // recibe la KEY confirmada
    Options() []map[string]string // pares ordenados {value: label}
}
```

Ejemplo de referencia: `devtui/example/HandlerSelection.go`
(`CompilerModeHandler`). **Zero coupling:** la interfaz usa solo tipos
stdlib — `server` NO importa `devtui`; la satisface estructuralmente y
`AddHandler(handler any, ...)` hace el type-assert en devtui (soporte ya
implementado en `devtui/handlerRegistration.go` / `anyHandler.go`).

Cambios en `tui.go`:

1. Constantes tipadas para las keys y labels (regla §1 — sin strings
   repetidos):

```go
const (
    execModeInternal = "internal"
    execModeExternal = "external"
)
```

2. `Options()` devuelve las dos opciones ordenadas
   (`{internal: Internal}, {external: External}`).
3. `Value()` devuelve la KEY activa (`execModeExternal` si
   `!executionInternal`), conservando el `strategyMu.RLock()` actual.
4. `Change(newValue)`: switch sobre la key — `execModeInternal` →
   `SetExternalServerMode(false)`; `execModeExternal` →
   `SetExternalServerMode(true)`; key desconocida → log del error, sin
   cambiar el modo (nada se traga en silencio). **Se elimina** el parser de
   pares `"Execution External:T"` (`Split(",")`/`Index(":")`) completo.
5. `Label()` pasa a nombrar el grupo (p. ej. `"Execution"`); `Name()` sigue
   `"SERVER"`; `SetLog`/`RefreshUI` quedan como están.
6. NO implementar `ShortcutProvider` (fuera de alcance; los botones bastan).

Tests (con `gotest`): reescribir `tests/modes_test.go` al contrato nuevo —
hoy asierta el formato viejo (`"Execution External:T"` en `Value()`/`Change`,
incluida la línea ~258):

1. `Value()` inicial = `internal`; tras `Change("external")` = `external` y
   el modo del server cambió; `Change("internal")` vuelve.
2. Key desconocida (`Change("bogus")`) → el modo NO cambia y se loguea error.
3. `Options()` devuelve exactamente 2 entradas ordenadas con keys
   `internal`/`external`.

## 6. Etapa 4 — documentación

- `docs/ARCHITECTURE.md`: sección de RBAC/rutas — documentar que la página
  default de dev y el contenido de `PublicDir` son públicos, y por qué eso no
  debilita el fail-closed de las rutas de negocio.

## 7. Criterios de aceptación

1. `gotest ./...` verde.
2. `GET /` anónimo en dev nunca responde 403: sirve index.html o el mensaje
   default.
3. RBAC fail-closed intacto para rutas de negocio (test §4.3).
4. `ServerHandler` satisface `devtui.HandlerSelection` sin importar `devtui`;
   el formato `"Execution External:T"` desaparece del repo (grep vacío).

## 8. Tabla de etapas

| # | Etapa | Archivos | Gate |
|---|-------|----------|------|
| 1 | Raíz pública que sirve index | `strategies.go` | tests §4.1–4.2 |
| 2 | Fallback estático en `/` | `httpd/static.go` (si opción b) | test §4.4 |
| 3 | `ServerHandler` → HandlerSelection | `tui.go`, `tests/modes_test.go` | tests §5.1–5.3 |
| 4 | Docs | `docs/ARCHITECTURE.md` | 1–3 |
