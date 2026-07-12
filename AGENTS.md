# AGENTS.md — `tinywasm/server`

Restricciones de este repo para cualquier agente (humano o LLM) que edite código aquí.

## Ubicación de tests: todos en `tests/`

Todo archivo `_test.go` vive en la carpeta global `tests/`, en la raíz del repo,
como paquete `<módulo>_test` (caja negra, importando el paquete público). Está
prohibido esparcir archivos `_test.go` sueltos junto al código que prueban.

**Única excepción:** un test que necesita acceso a identificadores **no
exportados** del paquete que prueba (structs/funciones privadas, campos internos).
Ese test **debe** vivir junto al código, como `package <paquete>` (caja blanca) —
nunca en `tests/`, porque `tests/` es un paquete `_test` externo y no tiene acceso
a lo no exportado.

- Ejemplo ya presente: `handle_file_event_test.go` (raíz, `package server`) — usa
  `externalStrategy` y `ErrUnsupportedEvent`, no exportados.
- Cuando un subpaquete (p. ej. `server/httpd`) tenga tests de caja blanca sobre sus
  propios no-exportados (`httpRouter`, `httpContext`), esos tests viven en el
  subpaquete (`server/httpd/adapter_test.go`), no en `tests/` — la regla de "junto
  al código si necesita lo interno" aplica por paquete, no solo en la raíz del repo.
- Todo lo demás — tests de comportamiento observable desde la API pública,
  integración, blackbox — va en `tests/`, sin importar cuántos paquetes toque.

Antes de añadir un test nuevo: si compila usando solo identificadores exportados,
va en `tests/`. Si necesita un identificador no exportado, va junto al paquete que
lo define — y solo ahí.

## Seguridad y Archivos Estáticos

- Para servir un archivo individual, usa `PublicAsset(path, handler)`.
- Para servir un directorio completo como fallback, usa `PublicDir(prefix, dir)`.
- Para servir archivos con permisos, usa `Get(...).Requires(...)`.
- **PROHIBIDO:** El uso directo de `http.FileServer` o `http.Dir` fuera del sistema de rutas del router. Todo acceso a archivos debe estar declarado y pasar por la verja de seguridad del router.
