> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_ADAPTER_MASTER_PLAN.md` — **Fase 1**.
> Depende de la **Fase 0** (`tinywasm/router` con cookies) ya publicada.

# Plan — `tinywasm/server/httpd`: subpaquete "enchufe simple" que reutiliza el adaptador existente

> **No se crea un módulo nuevo.** `router_adapter.go` (raíz del repo `tinywasm/server`)
> ya implementa el contrato completo de `github.com/tinywasm/router`
> (`httpRouter`/`httpContext`/`httpStreamer`/`httpRoute`, con cookies y `Routes()`
> ya resueltos). Falta exponerlo como librería pública y añadirle las "baterías" de
> servir HTTP (gzip, no-cache, estáticos, `/health`, RBAC). Este plan mueve ese
> adaptador a un **subpaquete** `github.com/tinywasm/server/httpd` (mismo módulo,
> mismo repo, mismo `go.mod`) y lo hace consumible tanto por el modo de desarrollo
> (`internalStrategy`, TUI/watch) como por binarios de producción (`mjosefa-cms` y
> por `templates/server_basic.md`, la plantilla que hoy genera a mano el mismo
> boilerplate cuando no existe `web/server.go` o se activa el modo `external`).

---

## Por qué subpaquete y no módulo nuevo

- **Cero colisión de nombres:** el paquete raíz `server` ya exporta `Config` y
  `New()` para `ServerHandler` (el orquestador de desarrollo con TUI/watch/
  compilación). Un subpaquete `httpd` tiene su propio namespace — `httpd.Config`,
  `httpd.New` — sin invadir la superficie existente.
- **Una sola implementación del adaptador.** Hoy `router_adapter.go` vive sin
  exportar en la raíz y solo lo usa `internalStrategy` (servidor en memoria del modo
  dev, `strategies.go:65-66`). Moverlo a `httpd` y hacer que `internalStrategy`
  lo importe desde ahí elimina cualquier riesgo de que las dos copias diverjan.
- **Mismo repo, mismo ciclo de release.** Los consumidores externos (`mjosefa-cms`)
  ya dependerán de `github.com/tinywasm/server`; añadir el subpaquete no exige un
  nuevo módulo en el registro ni un nuevo `AGENTS.md`.

---

## Reglas de Desarrollo

> Ver `AGENTS.md` (raíz del repo) para las restricciones generales del proyecto.
> La relevante aquí: **ubicación de tests** — todo lo que se pueda probar desde la
> API pública va en `tests/` (paquete `server_test`, caja negra); solo un test que
> necesite un identificador no exportado vive junto al código, y esa excepción
> aplica por paquete, no solo en la raíz del repo (ver más abajo).

- Todo el subpaquete `httpd/` lleva `//go:build !wasm`; aquí sí se permite stdlib
  (`net/http`, `compress/gzip`, `io`) — nunca compila a WASM.
- **`net/http` no aparece en la superficie pública** de `httpd`. Firmas exportadas
  usan `httpd.Config`, `httpd.Server`, `router.Router`, `router.APIModule` —
  nunca `http.ResponseWriter`/`http.Handler`/`*http.ServeMux`. `http.Server` queda
  encapsulado tras `ListenAndServe()`.
- **`internalStrategy` no cambia de comportamiento.** Solo se le reemplaza el
  constructor del router (`newHTTPRouter(mux)` → `httpd.NewRouter(mux)`); el bucle
  de arranque/parada/restart con `WaitGroup`/`ExitChan` de `strategies.go` se
  conserva intacto.
- **Tests del adaptador (`httpContext`/`httpRouter`/`httpRoute`) son caja blanca**
  (necesitan los no-exportados) → viven en `server/httpd/adapter_test.go`, junto al
  paquete `httpd`, no en `tests/`. **Tests de las baterías** (`middleware.go`,
  `static.go`, `enforce.go`, `routes_endpoint.go`, `httpd.go`) se prueban desde la
  API pública (`httpd.New(...)` + `httptest`) → van en `tests/`, como
  `tests/httpd_test.go` (paquete `server_test`).

---

## Estado de partida (en `router_adapter.go`, raíz del repo)

Ya implementado y con test (`router_adapter_test.go`) — **se mueve, no se reescribe**:

- `httpContext` completo: `Method/Path/Body/GetHeader/SetHeader/WriteStatus/Write/
  SetValue/Value/SetCookie/Cookie` (cookies con `mapSameSite` vía switch, sin enteros
  mágicos).
- `httpStreamer` (`Flush`).
- `httpRouter`: `Get/Post/Put/Delete/Options/Handle/Stream/Socket/Use/Routes`;
  aplica middlewares en orden inverso; `Socket` es stub `501`.
- `httpRoute.Requires` graba `(resource, action)` en un `router.RouteInfo` interno.
- Aserciones de contrato: `var _ router.Context = (*httpContext)(nil)`, etc.

**No implementado todavía (lo que aporta este plan):** las baterías de servir HTTP
(gzip, no-cache, estáticos, `/health`, enforcement RBAC, `/_routes`) y la fachada
`Config`/`New`/`Mount`/`ListenAndServe`.

---

## API pública objetivo

```go
//go:build !wasm

package httpd // github.com/tinywasm/server/httpd

import "github.com/tinywasm/router"

const (
	DefaultPort      = "8080"
	DefaultPublicDir = "public" // relativo: el binario corre con cwd=web/
	HealthPath       = "/health"
	RoutesPath       = "/_routes"
)

// Config declara el servidor. httpd NO lee argv/env — es responsabilidad del
// consumidor (main()) resolver Port/PublicDir/NoCache desde CLI args o entorno
// (vía tinywasm/env) y pasarlos ya resueltos aquí. Los ceros de Port/PublicDir
// son defaults documentados (ver DefaultPort/DefaultPublicDir); Gzip y Health
// no tienen "default silencioso" — el consumidor los fija explícitamente
// (normalmente Gzip:true, Health:true, fijos) porque casi nunca cambian entre
// entornos. NoCache sí varía por entorno (activado en dev, desactivado en
// producción) y por eso el consumidor lo resuelve dinámicamente — ver el
// ejemplo de uso más abajo.
type Config struct {
	Port      string
	PublicDir string // "" = no servir estáticos
	Gzip      bool
	NoCache   bool
	Health    bool
	Logger    func(...any)

	Identify       func(ctx router.Context) string
	Authorizer     func(userID, resource, action string) bool
	RoutesEndpoint bool

	// --- HTTPS: exactamente un modo activo, o ninguno (HTTP plano) ---
	TLS TLSConfig
}

// TLSConfig selecciona el modo HTTPS. Campos explícitos por modo — no un solo
// `Https bool` que adivine según qué otros campos vengan rellenos: cada modo
// tiene su propio conjunto de campos requeridos y New() valida al arrancar que
// como máximo uno esté activo (nunca en runtime).
type TLSConfig struct {
	// AutoCert: ACME/Let's Encrypt automático. Requiere Domain accesible desde
	// internet en :80/:443. Vía golang.org/x/crypto/acme/autocert (oficial Go).
	AutoCert bool
	Domain   string // requerido si AutoCert; New() falla si falta

	// CertFile/KeyFile: certificado ya emitido (CA propia/interna, o cualquier
	// CA), para producción sin salida a internet. Sin dependencias — stdlib
	// tls.LoadX509KeyPair. Ambos requeridos juntos.
	CertFile string
	KeyFile  string

	// DevTLS: self-signed local para desarrollo/offline. httpd genera una CA +
	// certificado local (stdlib, cacheados en disco) e instala la CA en el
	// almacén de confianza del SO/navegador (github.com/smallstep/truststore,
	// misma librería que usa mkcert) para que no salga warning. Requiere
	// permisos de sistema la primera vez (sudo/UAC) — ver "Fallback" abajo.
	DevTLS bool
}

type Server struct{ /* privado */ }

// New construye el servidor con Config (aplica defaults sobre los ceros).
func New(c Config) *Server

// Router expone el adaptador ya existente para registrar rutas directamente.
func NewRouter(mux *http.ServeMux) router.Router // exportado; lo reusa internalStrategy

func (s *Server) Router() router.Router
func (s *Server) Mount(m ...router.APIModule) *Server
func (s *Server) ListenAndServe() error
```

Uso objetivo (el mismo "enchufe simple" que ya usa `mjosefa-cms`). `Port`/`PublicDir`
se resuelven vía `env.Arg` con fallback al default (mismo patrón que ya usa el
`main()` generado hoy por `templates/server_basic.md`, `lookupArg`); `NoCache` se
resuelve igual — dinámico por entorno, no fijo — y `Gzip`/`Health` van fijos porque
no cambian entre entornos:

```go
port := env.Arg(argPort)
if port == "" {
	port = httpd.DefaultPort
}
publicDir := env.Arg(argPublicDir)
if publicDir == "" {
	publicDir = httpd.DefaultPublicDir
}
noCache := env.Arg(argNoCache) == "true" // default false: caché activada en producción

httpd.New(httpd.Config{
	Port: port, PublicDir: publicDir, Gzip: true, NoCache: noCache, Health: true,
}).
	Mount(mcpServer).
	ListenAndServe()
```

---

## Estructura interna (archivos del subpaquete `server/httpd/`)

- `adapter.go` — **contenido movido tal cual** desde `router_adapter.go` de la raíz
  (con su test). Único cambio: exportar el constructor como
  `func NewRouter(mux *http.ServeMux) router.Router` (hoy `newHTTPRouter`, no
  exportado). El resto (`httpContext`, `httpStreamer`, `httpRouter`, `httpRoute`,
  cookies, `Routes()`) se mantiene sin tocar.
- `httpd.go` — `Config`, `applyDefaults`, `New`, `Server`, `Router`, `Mount`,
  `ListenAndServe` (arma `*http.ServeMux`, cadena de baterías, `/health`, `/_routes`,
  `http.Server`).
- `middleware.go` — `gzip` y `noCache` como `router.Middleware` (operan sobre
  `router.Context`, no sobre `http.Handler`), portados 1:1 desde la lógica ya
  probada en `mjosefa-cms/web/server.go` y en `templates/server_basic.md`.
- `static.go` — sirve `Config.PublicDir` en `/` (envuelve `http.FileServer`
  internamente).
- `enforce.go` — por cada `RouteInfo` con `Resource != ""`: `Identify` → si
  `!Authorizer(userID, resource, action)` → `403`. **Diagnóstico ruidoso:** ruta
  protegida + `Authorizer == nil` → `New`/`ListenAndServe` devuelve error al
  arrancar, nunca en runtime.
- `routes_endpoint.go` — si `Config.RoutesEndpoint`, `GET /_routes` serializa
  `Router().Routes()` a JSON con `tinywasm/json`.
- `tls.go` — resuelve `Config.TLS` a un `*tls.Config` (o nil = HTTP plano) y
  decide qué `net.Listener`/método de arranque usar. **Valida al arrancar** (no
  en runtime) que como máximo un modo esté activo; error inmediato si:
  `AutoCert && Domain == ""`, o `(CertFile == "") != (KeyFile == "")`, o dos
  modos activos a la vez.
  - `AutoCert`: envuelve `golang.org/x/crypto/acme/autocert.Manager` (cache en
    disco vía `autocert.DirCache`); `ListenAndServe` usa
    `manager.Listener()`/`GetCertificate`.
  - `CertFile`/`KeyFile`: `tls.LoadX509KeyPair` (stdlib), sin dependencias.
  - `DevTLS`: `devcert.go` — genera CA local + certificado leaf para
    `localhost`/`127.0.0.1` (stdlib `crypto/x509`/`crypto/ecdsa`), cacheados en
    un dir local (p. ej. `~/.tinywasm/httpd/certs`, reutilizado entre arranques
    para no regenerar ni reinstalar cada vez). Instala la CA en el almacén de
    confianza del SO con `github.com/smallstep/truststore` (mismo mecanismo que
    `mkcert`, cross-platform: NSS/certutil en Linux, Keychain en macOS,
    CryptoAPI en Windows) — **una sola vez** (verifica antes si ya está
    instalada, evita pedir sudo/UAC en cada arranque). Si la instalación falla
    (sin permisos, entorno headless/CI): loguea el warning vía `Config.Logger`
    y sirve igual con el cert self-signed sin confiar — el navegador mostrará
    advertencia, pero el servidor funciona (nunca bloquea el arranque por esto).

## Cambio en el resto del repo (para eliminar la duplicación)

- `strategies.go:65-66` (`internalStrategy.Start`): cambiar
  `r := newHTTPRouter(mux)` por `r := httpd.NewRouter(mux)`, importando
  `github.com/tinywasm/server/httpd`. Sin más cambios en `strategies.go` — el
  bucle de arranque/parada/`ExitChan`/`WaitGroup` del modo dev no se toca.
- Eliminar `router_adapter.go` y `router_adapter_test.go` de la raíz del repo (ya
  movidos a `httpd/adapter.go` / `httpd/adapter_test.go`).
- `templates/server_basic.md` (usado por `generator.go` cuando no existe
  `web/server.go`, o al activar `SetExternalServerMode(true)`): reemplazar el
  boilerplate `net/http` (gzip/noCache/FileServer/mux/`/health`/`http.Server`,
  líneas 8-101 hoy) por:

  ```go
  //go:build !wasm

  package main

  import "github.com/tinywasm/env"
  import "github.com/tinywasm/server/httpd"

  const (
  	argPort      = "server_port"
  	argPublicDir = "server_public_dir"
  	argNoCache   = "server_no_cache"
  )

  func main() {
  	port := env.Arg(argPort)
  	if port == "" {
  		port = "{{.AppPort}}"
  	}
  	publicDir := env.Arg(argPublicDir)
  	if publicDir == "" {
  		publicDir = "{{.PublicDir}}"
  	}

  	httpd.New(httpd.Config{
  		Port:      port,
  		PublicDir: publicDir,
  		Gzip:      true,
  		NoCache:   env.Arg(argNoCache) == "true", // default false: caché activada
  		Health:    true,
  	}).ListenAndServe()
  }
  ```

  Conserva la posibilidad de sobreescribir `Port`/`PublicDir` por CLI arg que ya
  tenía el template legado (`lookupArg`, ahora `env.Arg`), y añade `server_no_cache`
  con el mismo mecanismo: `tinywasm/app` (u otro orquestador de dev) lo activa
  pasando `-server_no_cache=true` en `SetRunArgs` cuando `DevMode` está activo;
  en producción, sin el flag, `NoCache` queda `false` (caché normal del navegador).

  Esto es lo que verán los proyectos que **no** tienen `web/server.go` propio (o que
  activan modo `external` sin haberlo personalizado) — deja de generarse boilerplate
  copiado, se genera un enchufe a `httpd`.

---

## Pasos de implementación

1. Crear `server/httpd/` (subpaquete, mismo `go.mod`). Mover `router_adapter.go` →
   `httpd/adapter.go`, `router_adapter_test.go` → `httpd/adapter_test.go`;
   exportar `newHTTPRouter` → `NewRouter`. Ajustar el `package` de ambos archivos a
   `httpd`.
2. `strategies.go`: importar `github.com/tinywasm/server/httpd`; reemplazar
   `newHTTPRouter(mux)` por `httpd.NewRouter(mux)` en `internalStrategy.Start`.
   Correr los tests existentes del modo dev (`tests/modes_test.go`,
   `tests/startserver_integration_test.go`) para confirmar que no cambia el
   comportamiento.
3. `httpd/middleware.go`: portar `gzip`/`noCache` desde `templates/server_basic.md`
   / `mjosefa-cms/web/server.go`, como `router.Middleware`.
4. `httpd/static.go`: servir `PublicDir` en `/`.
5. `httpd/enforce.go`: envoltura RBAC por ruta + validación de arranque.
6. `httpd/routes_endpoint.go`: `GET /_routes` (gated por `Config.RoutesEndpoint`).
7. `httpd/httpd.go`: `Config` + defaults, `New`, `Router`, `Mount`,
   `ListenAndServe` (mux + baterías + `/health` + `/_routes` + `http.Server`).
8. `templates/server_basic.md`: reemplazar el boilerplate por el enchufe a
   `httpd` (ver arriba). Actualizar `tests/generator_test.go` si compara contenido
   generado con literal esperado.
9. `docs/ARCHITECTURE.md`: documentar `httpd` como el único implementador nativo
   de `router.Router` del ecosistema; su relación con `internalStrategy` (modo dev,
   lo consume) y con `templates/server_basic.md` (modo external/generado, lo usa).
10. `tests/httpd_test.go` (nuevo, `package server_test`): los 6 casos de
    `httptest` sobre las baterías (ver "Estrategia de pruebas"), montados vía
    `httpd.New(...)` — API pública, por eso van en `tests/` y no en `httpd/`
    (ver `AGENTS.md`).
11. `httpd/tls.go` + `httpd/devcert.go`: resolución de `Config.TLS` (los tres
    modos), validación de arranque (a lo sumo un modo activo; `AutoCert` exige
    `Domain`; `CertFile`/`KeyFile` van en pareja). `go.mod`: añadir
    `golang.org/x/crypto` (para `acme/autocert`) y `github.com/smallstep/truststore`.
12. `strategies.go`/`server.go`: el `Https bool` ya existente en `ServerHandler.Config`
    (hoy solo afecta `WaitForPortListening`/`OpenBrowser`, no sirve TLS de verdad)
    pasa a mapear a `httpd.TLSConfig{DevTLS: true}` cuando `internalStrategy` arma
    su servidor — así el modo dev también sirve HTTPS real vía `httpd`, no solo
    simula el esquema de la URL.

---

## Code Quality Checklist (obligatorio)

- **Sin literales repetidos → constantes tipadas.** `DefaultPort`, `DefaultPublicDir`,
  `HealthPath`, `RoutesPath`, cabeceras de cache-control, `Accept-Encoding`/`gzip`
  como constantes.
- **`net/http` fuera de la superficie pública de `httpd`.** Verificable:
  `grep 'http\.' ` sobre identificadores exportados no debe aparecer.
- **Fallar en arranque, no en runtime.** Ruta protegida sin `Authorizer` → error en
  `New`/`ListenAndServe`.
- **Una sola forma.** Único camino: `New(...).Mount(...).ListenAndServe()`.
- **`internalStrategy` sin duplicar el adaptador.** Debe importar `httpd.NewRouter`,
  no mantener una copia local.
- **`httpd` no importa nada de la raíz del paquete `server`** (evita import cycle:
  la raíz sí puede importar `httpd`, al revés no).
- **A lo sumo un modo TLS activo, validado en `New`.** `AutoCert && (CertFile != "" || DevTLS)`,
  o `Domain == ""` con `AutoCert`, o `CertFile`/`KeyFile` incompletos → error
  inmediato, nunca un `ListenAndServe` que falla a mitad de arranque.
  Nunca en runtime.
- **`DevTLS` nunca bloquea el arranque por no poder instalar la CA.** Fallo de
  `truststore` (sin permisos, headless) → warning por `Logger`, sirve igual con
  el cert sin confiar.

---

## Estrategia de pruebas y criterios de aceptación

> Ubicación regida por `AGENTS.md`: caja blanca (no-exportados) junto al paquete,
> todo lo demás en `tests/`.

- `gotest`. Solo nativo (`!wasm`).
- Aserciones de contrato ya existentes (caja blanca, `httpContext`/`httpRouter`
  internos) se conservan tal cual en `httpd/adapter_test.go`.
- Nuevos casos con `httptest` en `tests/httpd_test.go` (`package server_test`,
  vía API pública `httpd.New(...)`):
  1. `PublicDir` sirve un archivo con cabeceras no-cache cuando `NoCache:true`.
  2. `Gzip:true` + `Accept-Encoding: gzip` → `Content-Encoding: gzip` en la respuesta.
  3. `Health:true` → `GET /health` responde `200 "ok"`.
  4. Un `router.APIModule` de prueba montado con `Mount` recibe sus rutas.
  5. RBAC: ruta con `Requires("res","write")`; `Authorizer` que niega → `403`; que
     concede → `200`; `Authorizer==nil` + ruta protegida → error en `New`.
  6. `RoutesEndpoint:true` → `GET /_routes` devuelve JSON con `RouteInfo`; `false` → `404`.
  7. `TLS.CertFile`/`KeyFile` con un cert de prueba (generado en el test, stdlib) →
     servidor responde por HTTPS con ese certificado.
  8. Validación de arranque: `TLS.AutoCert:true` sin `Domain` → `New` devuelve
     error; dos modos TLS activos a la vez → error.
  9. `DevTLS` (aislado, `t.Skip` en CI sin permisos de sistema o headless):
     genera cert local y sirve HTTPS; no verifica instalación real en el
     truststore del CI, solo que el servidor arranca y sirve con el cert
     generado aunque la instalación falle (Logger recibe el warning).
- Tests existentes del modo dev (`tests/modes_test.go`, `tests/startserver_integration_test.go`,
  `tests/restart_on_fix_test.go`) siguen pasando sin cambios de comportamiento tras el
  paso 2.
- `templates/server_basic.md`: `tests/generator_test.go` actualizado para esperar el
  nuevo contenido (enchufe a `httpd`, no boilerplate `net/http`).

---

## Tabla de etapas

| Etapa | Archivo | Acción |
|---|---|---|
| 1 | `server/httpd/adapter.go` (+ test) | Mover `router_adapter.go`; exportar `NewRouter` |
| 2 | `strategies.go` | `internalStrategy` usa `httpd.NewRouter` en vez de la copia local |
| 3 | `server/httpd/middleware.go` | `gzip`/`noCache` como `router.Middleware` |
| 4 | `server/httpd/static.go` | Servir `PublicDir` en `/` |
| 5 | `server/httpd/enforce.go` | RBAC por ruta + validación de arranque |
| 6 | `server/httpd/routes_endpoint.go` | `GET /_routes` (gated) |
| 7 | `server/httpd/httpd.go` | `Config`+defaults, `New`, `Router`, `Mount`, `ListenAndServe`, `/health` |
| 8 | `templates/server_basic.md` | Reemplazar boilerplate por enchufe a `httpd` |
| 9 | `docs/ARCHITECTURE.md` | Documentar relación httpd ↔ internalStrategy ↔ template |
| 10 | `tests/httpd_test.go` | 9 casos `httptest` de las baterías + TLS, vía API pública (`AGENTS.md`) |
| 11 | `server/httpd/tls.go`, `server/httpd/devcert.go`, `go.mod` | 3 modos TLS; deps `x/crypto` + `smallstep/truststore` |
| 12 | `strategies.go`, `server.go` | `ServerHandler.Https` mapea a `httpd.TLSConfig{DevTLS:true}` (HTTPS real en modo dev) |
