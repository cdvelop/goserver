---
message: "feat!: httpd enforces the typed Access contract — guarded by default, contradictions fail at startup"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/AUTH_POLICY_MASTER_PLAN.md` — **Fase D**.
> Autocontenido: el agente que lo ejecuta no tiene contexto previo.
>
> **COMPUERTA:** requiere `tinywasm/model` **v0.0.12+** y `tinywasm/router` **v0.1.9+**
> (ambos publicados). Sube `go.mod` primero. **Sin ellos este repo NO COMPILA**: `httpd`
> implementa `router.Router` y el contrato cambió. Ese fallo de compilación es deliberado —
> el compilador rechaza a todo implementador que aún hable con strings.

# PLAN — `server`: aplicar el contrato de acceso tipado

**Dos cosas, y la segunda cambia el comportamiento a propósito:**

1. `httpd` deja de hablar de recursos y acciones con `string` pelados: usa `model.Resource`,
   `model.Action` y `model.Authorizer`.
2. **La verja pasa a leer UNA declaración (`model.Access`) en vez de deducirla por ausencia.**
   Esto **endurece el default**, y hay que hacerlo con los ojos abiertos (ver "El cambio de
   comportamiento").

---

## El problema (contexto, ya decidido — no lo reabras)

`Config.Authorize func(userID, resource, action string) bool` es la costura entre quien
**aplica** el acceso (este servidor) y quien lo **sabe** (un módulo de auth). Con tres
`string` seguidos, invertir dos argumentos compila, y el fallo no es un error: es una
**denegación silenciosa** en runtime.

Peor: la verja deducía el estado de acceso de una **combinación** — `Public bool` junto a un
`Resource` vacío-o-no. Eso hacía escribible un estado ilegal:

```go
r.Get("/orders", h).Public().Requires("orders", model.Read)  // compilaba
```

…y la verja se quedaba con `Public`, **descartando el permiso en silencio**: una ruta que
parecía protegida y no lo estaba.

`router` v0.1.9 ya lo cerró: `RouteInfo.Public` **desaparece** y en su lugar hay
`Access model.Access`, con tres estados y el **zero value = `AccessGuarded`**.

## Paso 1 — `httpd/adapter.go`: implementar el contrato nuevo

`httpRouter`/`httpRoute` implementan `router.Router` y `router.Route`. Cambios:

- `Requires(resource model.Resource, action model.Action)` — además de fijar recurso y
  acción, **fija `Access = model.AccessGuarded`**.
- **`Authenticated() router.Route` es un método NUEVO** del contrato: fija
  `Access = model.AccessAuthenticated`. Sin él, `*httpRoute` no compila como `router.Route`.
- `Public()` fija `Access = model.AccessPublic` (ya no un `bool`).
- `PublicAsset` / `PublicDir` fijan `Access = model.AccessPublic`. **No los toques por lo
  demás**: son otro contrato (servir archivos), ya cerrado y con sus propios tests.
- `RouteInfo.Resource` es `model.Resource`; `RouteInfo.Action`, `model.Action`.

## Paso 2 — `httpd/httpd.go`: la costura

```go
type Config struct {
	// …
	Authorize model.Authorizer // nil = toda ruta guarded queda denegada
}
```

## Paso 3 — la verja: un `switch`, no tres `if` por ausencia

En `applySecurityAndMiddleware` (`adapter.go`), sustituye la cadena actual por:

```go
switch route.info.Access {
case model.AccessPublic:
	next(ctx) // sin identidad: declarado a propósito

case model.AccessAuthenticated:
	if ctx.UserID() == "" { /* 403 */ }
	next(ctx)

default: // model.AccessGuarded — el zero value: identidad Y permiso
	if ctx.UserID() == "" { /* 403 */ }
	// model.Allowed deniega si Authorize es nil: la ausencia de respuesta no es permiso.
	if !model.Allowed(authorizer, ctx.UserID(), route.info.Resource, route.info.Action) {
		/* 403 */
	}
	next(ctx)
}
```

## Paso 4 — `httpd/enforce.go`: la contradicción falla al ARRANCAR

`validateRBAC` hoy solo comprueba "recurso sin `Authorize`". Amplíalo para rechazar toda
declaración que se contradiga — **al arrancar, nunca como sorpresa en runtime** (es el mismo
criterio que `mcp.AddTool` ya aplica a los tools):

| Situación | Por qué es fatal |
|---|---|
| `AccessGuarded` **sin** `Resource` | autorizaría contra `""` y **denegaría todas las llamadas**: una ruta que parece protegida y es inalcanzable, sin un solo error. |
| `AccessGuarded` **sin** `Authorize` configurado | igual que hoy: no hay quien responda. |
| **No** `AccessGuarded` **con** `Resource` | un recurso que nadie comprueba **parece protección y no la da**. |

## ⚠️ El cambio de comportamiento (léelo: es el corazón del plan)

Hasta ahora, **una ruta sin anotar era "cualquier identidad pasa"** (la tercera rama de la
verja: denegaba al anónimo y dejaba entrar a cualquier logueado). Ese default era demasiado
laxo: un `Get` que nadie anotó quedaba abierto a todo usuario autenticado **por omisión**.

Con el contrato nuevo, una ruta sin anotar cae en el **zero value `AccessGuarded`** y, al no
declarar recurso, **el arranque falla** (paso 4).

**Eso es exactamente lo que se busca**: lo que no se declara, no se sirve. Si alguna ruta de
este repo necesita el comportamiento antiguo, **decláralo** con `.Authenticated()`. No
relajes la verja, y **no inventes un recurso** para "callar" el error.

> `GET /health` se registra directamente sobre el `mux` (`httpd.go`), no a través del router,
> así que no pasa por la verja y **no necesita anotación**. Verifícalo, no lo asumas.

## Paso 5 — los guards

Los tests que ya existen (`PublicAsset`/`PublicDir` públicos, `PublicDir` visible en
`Routes()`, sin doble gzip, el `Content-Type` correcto) **deben seguir verdes**. Añade:

1. **El default deniega**: ruta con `.Requires(res, act)` y un llamador **sin permiso** → 403;
   con permiso → 200.
2. **`.Authenticated()`**: anónimo → 403; con identidad → 200 **sin consultar `Authorize`**
   (compruébalo con un `Authorizer` que registre si fue llamado: **no debe serlo**).
3. **`.Public()`** → anónimo → 200.
4. **`Authorize` nil + ruta guarded** → **403**, no 200. La ausencia de respuesta no es permiso.
5. **El arranque falla** en las tres contradicciones del paso 4 (`Handler()` devuelve error).

---

## ⚠️ Anti-footguns

- **NO debilites el cierre por defecto** para que algo compile. Si una ruta da 403, la
  respuesta no es abrir la verja: es que esa ruta debe **declarar** lo que necesita.
- **NO inventes un recurso** (`"api"`, `"default"`) para silenciar el error del paso 4. El
  vocabulario lo declara el consumidor, jamás esta librería.
- **NO toques** el fallback de `static.go` ni la batería `Gzip` más allá de lo que pida el
  tipo: son otro contrato, ya cerrado y con sus propios tests de regresión.
- **NO conviertas a `string`** para esquivar el tipo.
- Nunca ejecutes `gopush` ni `codejob`.

## Criterios de aceptación

```bash
go build ./...
grep -rn "resource, action string\|Public\s*bool" --include=*.go .   # → vacío
gotest                                                               # verde
```

## Consecuencia fuera de este repo (no es tu trabajo)

`app` y `goflare/edge` también consumen el contrato y están en rojo. Coordinado en el master
plan: **no toques esos repos**.

## Al cerrar

Vuelca a `AGENTS.md` la regla permanente: *"el acceso es UNA declaración (`model.Access`), no
una combinación de banderas. El zero value es `AccessGuarded`: identidad y permiso. Una
contradicción (guarded sin recurso, o recurso sin guarded) falla al arrancar, nunca en
runtime. Un `Authorize` nil deniega."*

**No borres ni renombres este archivo.** El ciclo de vida de `PLAN.md`/`CHECK_PLAN.md` lo
gestiona `codejob`: lo renombra a `CHECK_PLAN.md` y lo elimina al cerrar el loop, leyendo su
frontmatter (`message:`, `tag:`) para el commit de cierre. Si lo borras, el loop no cierra.
