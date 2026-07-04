> Continuación de `docs/CHECK_PLAN.md` (ya ejecutado, ver auditoría — 11/12 pasos
> completos, build/vet/test verdes). Este plan cierra lo que quedó pendiente:
> cobertura de tests faltante en `httpd` y, adicionalmente, pruebas de
> concurrencia para dar por lista la librería para producción.

# Plan — Cerrar cobertura de tests de `tinywasm/server/httpd`

---

## Excepción de ubicación aceptada

`AGENTS.md`/`CHECK_PLAN.md` piden que las pruebas de "baterías" (todo lo que se
monta vía API pública: `httpd.New(...)`) vivan en caja negra, `package
server_test`, bajo `tests/`. En la práctica `httpd` **no expone ningún tipo
`net/http`** en su superficie pública (es la regla más importante del paquete,
ver checklist de `CHECK_PLAN.md`), así que no hay forma de levantar el
`http.Handler` real desde fuera del paquete sin abrir un socket real y pegarle
por HTTP.

**Decisión (aceptada por el usuario):** las pruebas de baterías se quedan donde
están hoy, `httpd/batteries_test.go`, en `package httpd` (caja blanca), en vez
de moverlas a `tests/`. Este plan solo **añade** los casos que faltan al mismo
archivo (o a nuevos archivos `_test.go` dentro de `httpd/`), no reubica nada.

---

## Estado de partida

`httpd/batteries_test.go` (`TestHTTPDBatteries`) ya cubre, todo en un único test
monolítico que arma el handler a mano (sin pasar por `ListenAndServe`):

1. `/health`
2. Estático + `NoCache`
3. Gzip
4. RBAC — permitir
5. RBAC — denegar (403)
6. `/_routes` — habilitado

**Huecos identificados en la auditoría de `CHECK_PLAN.md`:**

- `Mount(...router.APIModule)` — nunca se prueba que un módulo externo reciba
  sus rutas.
- `Authorizer == nil` + ruta protegida → error de arranque (`validateRBAC` vía
  `ListenAndServe`) — no probado.
- `RoutesEndpoint: false` → `GET /_routes` debe dar `404` — no probado (solo se
  prueba el caso `true`).
- Los 3 modos TLS (`CertFile`/`KeyFile`, validación de arranque, `DevTLS`) — no
  probados en absoluto; `tests/https_test.go` solo ejercita el helper legado
  `WaitForPortListening` contra un `httptest` genérico, no el código TLS de
  `httpd`.
- **Concurrencia** — ningún test ejercita el servidor bajo carga concurrente.
  Antes de llamarla lista para producción, la librería necesita demostrar que
  no hay data races ni corrupción de estado bajo uso concurrente real
  (`go test -race`).

---

## Reglas de Desarrollo

- Todo vive en `httpd/*_test.go`, `//go:build !wasm` implícito (el paquete
  entero ya lo es).
- Todo el paquete debe pasar con `go test -race ./...` — no solo `go test`.
- Los tests que arrancan un servidor real (TLS, concurrencia) usan
  `s.config.Port = "0"` no es viable porque `ListenAndServe()` no expone el
  puerto asignado por el SO (limitación conocida y documentada más abajo, ver
  "Deuda técnica"). Mientras tanto, estos tests fijan un puerto libre obtenido
  de antemano con `net.Listen("tcp", ":0")` + `Close()` inmediato (patrón
  estándar de Go para tests, hay una ventana de carrera teórica pero aceptable
  para CI) y arrancan el servidor en una goroutine.
- Ningún test debe bloquear el proceso: los servidores arrancados con
  `ListenAndServe()` en goroutine no tienen forma pública de detenerse (no hay
  `Shutdown()`) — los tests los dejan corriendo y confían en que el proceso de
  test termine (mismo patrón ya usado, implícitamente, por cualquier test que
  quisiera probar un puerto real). Ver "Deuda técnica" para la solución
  correcta a mediano plazo.

---

## Pasos de implementación

1. **`httpd/mount_test.go`** — `TestMount`: un `fakeAPIModule` con
   `MountAPI(r router.Router)` que registra `GET /mounted`; `s := New(cfg);
   s.Mount(fakeAPIModule{})`; verificar con una petición sintética (a través del
   mismo patrón manual que ya usa `TestHTTPDBatteries`, armando el handler con
   `s.wrapWithBatteries(s.mux)`) que la ruta responde.

2. **`httpd/enforce_test.go`** — `TestRBACMissingAuthorizerFailsAtStartup`:
   registrar una ruta con `.Requires("res","action")`, no configurar
   `Authorizer`, llamar `s.ListenAndServe()` en un puerto ya ocupado (o mejor:
   llamar directamente a `s.validateRBAC()` ya que es lo que `ListenAndServe`
   invoca primero, evitando el problema de puertos) y afirmar que devuelve
   error no nil con el recurso en el mensaje.

3. **`httpd/routes_endpoint_test.go`** — `TestRoutesEndpointDisabled`:
   `Config{RoutesEndpoint: false}`, pegarle a `/_routes` a través del handler
   armado a mano, afirmar `404` (comportamiento nativo de `http.ServeMux`
   cuando no se registró el patrón).

4. **`httpd/tls_test.go`**:
   - `TestValidateTLS_RejectsMultipleModes`: `TLS{DevTLS: true, CertFile: "a",
     KeyFile: "b"}` → `validateTLS()` error.
   - `TestValidateTLS_AutoCertRequiresDomain`: `TLS{AutoCert: true}` sin
     `Domain` → error.
   - `TestValidateTLS_CertKeyMustBePaired`: solo `CertFile` sin `KeyFile` →
     error.
   - `TestTLS_CertFileKeyFile_Serves`: generar cert+key de prueba en el test
     (stdlib `crypto/tls`/`crypto/x509`/`crypto/ecdsa`, self-signed, escritos a
     `t.TempDir()`), arrancar `ListenAndServe()` en goroutine con puerto libre
     pre-reservado, cliente `http.Client{Transport: &http.Transport{TLSClientConfig:
     &tls.Config{InsecureSkipVerify: true}}}` pega a `https://127.0.0.1:<puerto>/health`
     y espera `200`.
   - `TestDevTLS_ServesWithoutBlockingOnTruststoreFailure` — `t.Skip` si
     `testing.Short()` o si no hay entorno gráfico/permite de instalar
     truststore (CI headless): arrancar con `TLS{DevTLS: true}`, verificar que
     el server sirve por HTTPS igual aunque la instalación de la CA falle
     (capturar el warning vía `Config.Logger`).

5. **`httpd/concurrency_test.go`** — pruebas de concurrencia para producción
   (ver detalle abajo).

6. Correr `go test -race ./...` en el módulo completo; corregir cualquier data
   race real que aparezca (no silenciar con `-race` deshabilitado).

---

## Pruebas de concurrencia (producción)

Objetivo: demostrar que `httpd.Server` es seguro bajo tráfico concurrente real
y bajo registro/lectura concurrente de rutas, que es el patrón de uso esperado
(`Mount` se puede llamar desde múltiples inicializadores; el router se lee en
cada request).

1. **`TestConcurrentRequests_NoRace`** — arrancar un servidor real (puerto
   libre pre-reservado, `ListenAndServe()` en goroutine) con: una ruta estática
   servida desde `PublicDir`, una ruta API normal, una ruta con `Requires` +
   `Authorizer`, gzip y health habilitados. Lanzar N (p. ej. 200) goroutines de
   cliente que hacen peticiones concurrentes mezclando los cuatro tipos de ruta
   (`/health`, `/test.txt`, `/api/hello`, `/api/secret` con y sin header de
   usuario válido). Correr con `go test -race`; el objetivo es que el detector
   de razas no dispare, no solo que las respuestas sean `200`/`403` correctas.

2. **`TestConcurrentRouteRegistration`** — caja blanca sobre `httpRouter`
   (ya vive en `httpd/adapter_test.go` o un nuevo `adapter_concurrency_test.go`):
   registrar rutas (`Get`/`Post`/etc.) desde varias goroutines concurrentes
   *antes* de servir tráfico, y simultáneamente desde otra goroutina llamar
   `Routes()` en bucle corto. Confirma que `routes []route` (el slice interno)
   no se lee/escribe sin sincronización — si `-race` dispara aquí, hay que
   añadir un `sync.RWMutex` a `httpRouter` (cambio de código real, no solo de
   test, documentado como hallazgo si aplica).

3. **`TestConcurrentAuthorizerCalls`** — múltiples goroutinas golpeando una
   ruta protegida simultáneamente con distintos `X-User`, confirmando que
   `Authorizer` (función provista por el consumidor, potencialmente con estado
   compartido como un mapa de permisos) se invoca de forma segura y que
   `setAuthorizer` (llamado una sola vez desde `ListenAndServe`) no compite con
   las lecturas que hace cada request — si `Identify`/`Authorizer` se guardan
   en campos del `httpRouter` sin mutex y se leen por request, `-race` lo
   revela aquí.

4. **`TestGracefulUnderLoad`** *(opcional, solo si se resuelve la deuda técnica
   de `Shutdown()` en el punto 2 de "Deuda técnica")*: enviar tráfico
   concurrente y disparar `Shutdown()` a mitad de carga, confirmar que las
   conexiones en vuelo terminan limpio y las nuevas son rechazadas. Bloqueado
   hasta que exista `Shutdown()` en la API pública.

**Criterio de aceptación de esta sección:** `go test -race ./httpd/...` limpio,
cero warnings del detector de razas, en al menos 3 corridas consecutivas (las
razas son no determinísticas — una corrida limpia no es suficiente evidencia).

---

## Deuda técnica identificada durante este plan (no bloquea la sección de tests, pero limita qué se puede probar)

1. **No hay forma de obtener el puerto real asignado** cuando `Config.Port` es
   `"0"` o cuando se usa un listener externo — `ListenAndServe()` construye y
   descarta el `net.Listener` internamente. Los tests de esta sección
   sortean esto reservando el puerto por fuera (`net.Listen` + `Close()`
   inmediato), lo cual tiene una ventana de carrera teórica entre procesos.
   Solución correcta a mediano plazo: exponer `func (s *Server) Addr() string`
   (poblado después de que el listener interno bindee) sin filtrar
   `net.Listener`/`http.*` en la firma — compatible con la regla de "sin
   `net/http` en la superficie pública".
2. **No hay `Shutdown()` / `Close()` público.** Los servidores arrancados en
   goroutine durante los tests quedan corriendo hasta que el proceso de test
   termina. Para producción real (no solo tests) esto también es una limitación
   operativa: un consumidor de `httpd` no tiene manera de apagar el servidor
   ordenadamente (p. ej. en un `SIGTERM` de contenedor). Se recomienda añadir
   `func (s *Server) Shutdown(ctx context.Context) error` que delegue a
   `http.Server.Shutdown` internamente (sin exponer el tipo `http.Server`).
   Este punto es candidato a su propio mini-plan si el usuario lo prioriza.
