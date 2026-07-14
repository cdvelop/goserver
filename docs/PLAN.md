---
message: "fix: httpd registers method-aware patterns and proves conformance to the router contract"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_CONFORMANCE_MASTER_PLAN.md` — **Fase B**.
> **Requiere la Fase A publicada**: `tinywasm/router` con el paquete `conformance`.

# PLAN — `httpd`: registrar por método, y demostrar conformidad

Autocontenido, en español.

## El problema

Registrar tres métodos sobre el mismo path **hace panic al arrancar**:

```go
r.Post("/api/contacto", h).Public()
r.Get("/api/contacto", l).Public()
r.Options("/api/contacto", h).Public()
```

```
panic: pattern "/api/contacto" (registered at httpd/adapter.go:208) conflicts with
pattern "/api/contacto": /api/contacto matches the same requests as /api/contacto
```

La causa está en `httpd/adapter.go`, en `register`:

```go
func (r *httpRouter) register(route *httpRoute) {
	r.mux.HandleFunc(route.info.Path, func(w http.ResponseWriter, req *http.Request) {
		if route.info.Method != "" && req.Method != route.info.Method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		...
```

El patrón que va al `ServeMux` es **solo el path**: el método se filtra *dentro* del handler.
Así que dos rutas con el mismo path son **el mismo patrón**, y Go 1.22+ hace panic por patrón
duplicado. Cada path admite exactamente **un** método, y el contrato `router.Router` —que
ofrece `Get`/`Post`/`Put`/`Delete`/`Options` sobre cualquier path— es mentira en esta
implementación. En `goflare/edge` esas tres rutas conviven.

Un consumidor real (`goflare-demo`) ya había pagado la deuda: escribió una sola ruta
`Handle("", path)` con un `if ctx.Method() == "GET"` a mano dentro, re-implementando el
enrutado por método que la librería dice ofrecer.

## Cambios

### 1. `httpd/adapter.go` — el método va en el patrón

`ServeMux` soporta patrones con método desde Go 1.22 (`"GET /api/contacto"`), y este repo ya
está en Go 1.25. Es la herramienta correcta y estaba ahí.

En `register` (y en `registerStream`, que tiene el mismo defecto), construye el patrón con el
método cuando lo hay:

```go
// pattern builds the ServeMux pattern for a route. The method MUST be part of it:
// registering by path alone makes two routes on the same path the SAME pattern, and
// ServeMux panics on the duplicate. Go 1.22+ matches "GET /path" natively.
// An empty method means "any method" and stays a bare path pattern.
func pattern(info router.RouteInfo) string {
	if info.Method == "" {
		return info.Path
	}
	return info.Method + " " + info.Path
}
```

Y borra el filtro de método de dentro del handler: ya no puede dispararse —el `ServeMux` no
enruta ahí un método que no coincide— y dejarlo es código muerto que miente sobre quién
decide. Con el patrón por método, **el 405 lo emite el propio `ServeMux`**.

Ojo con dos cosas, ambas comprobadas por la suite de conformidad:

- Un patrón `"GET /path"` matchea **también `HEAD`**, por diseño de `ServeMux`. Es correcto.
- Una ruta con método `""` (`Handle("", path, h)`) sigue siendo legal y matchea cualquier
  método: **no la elimines**, el contrato la admite.

### 2. `httpd` demuestra conformidad

Crea `tests/conformance_test.go` (o el paquete de tests que ya use el repo):

```go
func TestHTTPDConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		New:    newHTTPDFactory,  // levanta el router real + un httptest.Server
		Verify: verifyStartup,    // enforce.go: guarded sin Authorize => error
	})
}
```

**El `serve` de la factory debe conducir la petición por la tubería REAL** —`ServeMux`, verja
RBAC, middlewares—, no llamar al handler a pelo. Un `serve` que se salta la verja prueba
justo lo contrario de lo que dice probar. La identidad (`userID`) que recibe `serve` se
inyecta por el mismo asiento que en producción: el middleware `Config.Authn`.

`Verify` conecta lo que `enforce.go` ya hace: una ruta `Guarded` sin `Config.Authorize`
configurado es un error de arranque. Este repo **ya cumple** ese punto del contrato; el plan
solo lo pone bajo el arnés para que siga cumpliéndolo.

## Anti-footguns

- **Este repo es backend nativo y usa `net/http` legítimamente.** No "arregles" esos imports:
  la regla de "nada de stdlib" es para paquetes que compilan a WASM, y `httpd` no lo hace.
  Lo que sigue prohibido es **exportar** tipos `http.*` en firmas del adaptador.
- **No toques la verja RBAC ni `model.Authorizer`.** Están bien y son la referencia que el
  resto del ecosistema debe alcanzar. Este plan **no** cambia el modelo de acceso.

## Criterios de aceptación

- `gotest` pasa.
- `TestHTTPDConformance` pasa: los 16 casos del contrato, en verde.
- Registrar `Get` + `Post` + `Options` sobre el mismo path **no hace panic**, y cada uno
  ejecuta su handler.
- Un método no registrado sobre un path que sí existe devuelve **405**.
- El filtro de método dentro del handler ha desaparecido:
  ```bash
  grep -rn "Method Not Allowed" httpd/    # → vacío: el 405 lo emite ServeMux
  ```
- El patrón se construye en un solo sitio:
  ```bash
  grep -rn "mux.HandleFunc" httpd/        # → siempre con pattern(...), nunca con info.Path pelado
  ```

## ⚠️ Publicación

Hay una decisión en pie de **no publicar `server` hasta arreglar la cascada de `gopush`**
(ver `devflow`). Implementa y deja verde; **la publicación se decide aparte**, y la Fase D
del master plan (`goflare-demo`) depende de ella.
