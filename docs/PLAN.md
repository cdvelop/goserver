---
message: "feat: implement PublicAsset/PublicDir — static files enter the permission gate"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/PUBLIC_ASSETS_MASTER_PLAN.md` — **Fase B1**.
> Autocontenido: el agente que lo ejecuta no tiene contexto previo.

# PLAN — `server`: servir archivos entra por la verja de permisos

**En una frase:** `httpd` implementa los dos métodos nuevos del contrato de `router`, y
**retira el `http.FileServer` que hoy sirve `web/public` por fuera del router**.

**Estado hoy: el repo NO COMPILA.** `tinywasm/router` **v0.1.7** añadió dos métodos a la
interfaz `Router`, y `httpRouter` ya no la satisface. Ese fallo de compilación es
deliberado: todo implementador debe declarar cómo sirve archivos.

---

## El problema (léelo antes de tocar nada)

El router es **privado por defecto**: una ruta que no declara `Public()` ni `Requires()`
deniega a quien no tiene identidad (`httpd/adapter.go`, la verja al final de
`applySecurityAndMiddleware`). Eso es correcto y **no se toca**.

Pero hoy conviven **dos regímenes de archivos**, y solo uno pasa por esa verja:

1. **Archivos generados en memoria** (`index.html`, `style.css`, `script.js`,
   `client.wasm`) → registrados como rutas normales con `r.Get(...)`. **Sí** pasan por la
   verja. Y como un navegador que pide `index.html` **siempre es anónimo**, recibía **403**:
   build correcto, página en blanco. (Esto se arregla en `assetmin` y `client`, no aquí.)

2. **El directorio `web/public`** → servido por un `http.FileServer` colgado como *fallback*
   **fuera del router** (`httpd/static.go`, `wrapWithBatteries`): si el mux responde 404, se
   intenta el archivo en disco. **Nunca entra en el sistema de permisos.** Es público *por
   accidente*, no por declaración — y eso es lo que este plan arregla.

Dos regímenes de seguridad en el mismo servidor, ninguno visible en una firma.

## La decisión (no la reabras)

Se descartó añadir `.Public()` a cada ruta de archivos: es un *"no olvides llamar a X"*,
y el olvido **no falla en compilación ni hace ruido** (403 silencioso). Ya ocurrió en tres
repos. En su lugar, `router` v0.1.7 declara:

```go
// UN archivo, UNA ruta. Público por construcción. No devuelve Route: no hay
// permiso que colgarle → no se puede olvidar abrirlo ni cerrarlo por error.
PublicAsset(path string, h HandlerFunc)

// Un directorio bajo un prefijo. Mismo contrato.
PublicDir(prefix string, dir string)
```

Y `RouteInfo` ganó el campo `Dir string` (vacío salvo en `PublicDir`), para que un
directorio servido sea **visible a la introspección** en vez de colarse por fuera.

Servir un archivo **con permisos** ya está cubierto y no necesita nada nuevo:
`r.Get("/factura/:id", h).Requires("invoices", "read")`. Si olvidas el `Requires`, la ruta
queda **privada**. La asimetría es deliberada: *abrir* necesita método propio porque el
olvido es silencioso; *cerrar* ya falla seguro.

---

## Paso 1 — `httpd/adapter.go` implementa los dos métodos

Sube `github.com/tinywasm/router` a **v0.1.7** o superior en `go.mod`.

Junto a `Get`/`Post`/… (que **no cambian**), añade:

```go
// PublicAsset registra un archivo servido al navegador: público por construcción.
func (r *httpRouter) PublicAsset(path string, h router.HandlerFunc) {
	route := &httpRoute{
		info: router.RouteInfo{Method: http.MethodGet, Path: path, Public: true},
		h:    h,
	}
	r.mu.Lock()
	r.routes = append(r.routes, route)
	r.mu.Unlock()
	r.register(route)
}
```

`Public: true` hace que la verja de `applySecurityAndMiddleware` lo deje pasar por la rama
que ya existe (`if route.info.Public { next(ctx); return }`) — **no toques esa verja**.

`PublicDir` se implementa en el paso 2, porque es donde vive su sustancia.

## Paso 2 — `PublicDir` sustituye al `FileServer` oculto

Este es el corazón del plan.

1. **Implementa `PublicDir`** en `httpd/adapter.go`: registra una ruta con
   `Method: GET`, `Path: prefix`, `Public: true`, `Dir: dir`, cuyo handler sirve el archivo
   pedido desde `dir`. Reutiliza la lógica que hoy vive en `httpd/static.go` (resolver la
   ruta, `filepath.Clean`, comprobar que existe y no es directorio, caer a `index.html` si
   lo es) — **cópiala, no la reinventes**, y **conserva la protección de path traversal**:
   el archivo resuelto debe seguir estando dentro de `dir`.

2. **Retira el fallback.** En `httpd/httpd.go:107`, `Handler()` envuelve el mux con
   `s.wrapWithBatteries(handler)`. Esa función mezcla dos cosas distintas:
   - el **servidor de estáticos** (`http.FileServer` sobre `config.PublicDir`) → **se
     elimina**: ya no se sirve por fuera del router.
   - las **"global batteries"** (compresión, cabeceras, etc.) → **se conservan**; sepáralas
     para que sigan aplicándose al mux.

3. **Registra el directorio como ruta declarada.** Donde hoy el `Server` conoce
   `config.PublicDir` (`httpd/httpd.go:19`, default `"public"`), si no está vacío debe
   registrar `r.PublicDir("/", s.config.PublicDir)` sobre su propio router, en vez de
   colgar un FileServer aparte.

**Resultado:** `Routes()` enumera el directorio servido. Antes era invisible.

## Paso 3 — `strategies.go`: el handler `/` por defecto

En `strategies.go` (~línea 84) el handler por defecto que sirve `PublicDir/index.html` está
registrado como `r.Get("/", ...).Public()`. Cámbialo a `r.PublicAsset("/", ...)` y quita el
`.Public()`: sirve un archivo, luego es un asset. **El cuerpo del handler no cambia.**

---

## ⚠️ Anti-footguns (NO hagas esto)

- **NO debilites la verja.** `Get`/`Post`/… siguen siendo privados por defecto. Si algo
  responde 403, la respuesta **no** es abrir la verja: es que esa ruta debía declararse
  `PublicAsset`.
- **NO dejes ningún `http.FileServer`, `http.Dir` ni regla por prefijo** que sirva archivos
  fuera del router. Es exactamente el bug que este plan retira.
- **NO añadas `.Public()` a rutas de archivos.** Si lo estás buscando, quieres `PublicAsset`.
- **NO toques** `assetmin` ni `client`: migran con sus propios planes (Fases C1 y C2).
- Nunca ejecutes `gopush` ni `codejob`.

## Criterios de aceptación (verificables)

```bash
# 1. El contrato se satisface y todo compila
go build ./...

# 2. Nada sirve archivos por fuera del router
grep -rn "FileServer\|http.Dir(" .        # → vacío

# 3. Ninguna ruta de archivos usa el marcador olvidable
grep -rn '\.Public()' .                    # → vacío en rutas de assets

# 4. Verde
gotest
```

**Tests que definen el plan** (escríbelos, usando `github.com/tinywasm/router/mock`):

1. `PublicAsset("/style.css", h)` → aparece en `Routes()` con `Public == true`.
2. `PublicDir("/", "web/public")` → aparece en `Routes()` con `Public == true` y
   `Dir == "web/public"`. **Antes de este plan el directorio no aparecía en `Routes()` en
   absoluto**; ese es el punto.
3. **Un archivo de `web/public` se sirve con 200 a un llamador sin identidad** (levanta el
   `Server` con `httptest`, sin authorizer). Es la regresión que importa: si alguien vuelve
   a meter el FileServer fuera del router, este test no lo detecta — pero si alguien rompe
   el `PublicDir`, sí.
4. **`Get("/api/x", h)` sin anotar sigue devolviendo 403** a un anónimo. La verja no se tocó.

## Al cerrar

Vuelca a `AGENTS.md` la regla permanente: *"para servir un archivo, `PublicAsset`; para un
directorio, `PublicDir`; para un archivo con permisos, `Get(...).Requires(...)`. Nunca un
FileServer fuera del router."* Luego **borra este `docs/PLAN.md`**.
