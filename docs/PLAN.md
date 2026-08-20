---
PLAN: "fix: server_external_mode se escribía y no se leía nunca"
EXECUTOR: jules
REVIEWER: none
---

> Plan autocontenido: todo lo necesario para ejecutarlo está aquí.
> Se despacha con el flujo CodeJob. Ver skill: agents-workflow.
>
> ⚠️ **Orden obligatorio:** este plan va **después** de
> `tinywasm/app/docs/PLAN.md`. Motivo en §4.

# Plan — una sola fuente de verdad para el modo de servidor

## 1. El defecto

`StoreKeyExternalServer` aparece exactamente tres veces en todo el repo:

```
server.go:42       StoreKeyExternalServer = "server_external_mode"   ← definición
server.go:314      _ = h.Store.Set(StoreKeyExternalServer, "true")   ← escritura
management.go:21   _ = h.Store.Set(StoreKeyExternalServer, "true")   ← escritura
```

**Ni un solo `Get`.** El valor se escribe en el `.env` del proyecto y **nunca**
se vuelve a leer.

Consecuencia observada en un proyecto real: su `.env` dice

```
server_external_mode=true
```

y el demonio arranca el servidor **interno**. El usuario lee un ajuste que
afirma algo que no manda nada: peor que no tener el ajuste.

## 2. Quién decide de verdad hoy

`management.go:11-25`, en `StartServer`:

```go
serverFilePath := filepath.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)
if _, err := os.Stat(serverFilePath); err == nil && h.executionInternal {
	h.log("Found existing server file, switching to External Server Mode")
	h.executionInternal = false
	h.strategy = newExternalStrategy(h)
	if h.Store != nil {
		_ = h.Store.Set(StoreKeyExternalServer, "true")   // ← escribe un valor que nadie lee
	}
}
```

La decisión real es **la existencia del archivo de servidor**. Eso está bien: es
explícito, está en el repositorio, se ve en un `ls` y no se puede desincronizar.
Lo que sobra es la copia en el store.

## 3. El cambio

**Borrar la constante y sus dos escrituras.** El archivo manda; punto.

- `server.go:42` — borrar `StoreKeyExternalServer`.
- `server.go:314` — borrar la escritura de `SetExternalServerMode`.
- `management.go:21` — borrar la escritura de `StartServer` (y el `if h.Store != nil`
  que la envuelve, si queda vacío).

Verificable: `grep -rn "server_external_mode\|StoreKeyExternalServer" .` →
**vacío**.

### Si `Store` se queda sin usuarios

Comprueba si `ServerHandler.Store` sirve para algo más:

```sh
grep -rn "h.Store\|\.Store\b" --include="*.go" . | grep -v _test
```

Si esas eran sus únicas escrituras y no hay lecturas, **quita también el campo
`Store`, `noopStore` y el setter que lo inyecta**: superficie exportada que solo
usa la propia librería es plomería expuesta
(`docs/CONSTRUCTION_HARNESS.md`, principio 5). Si algún consumidor lo inyecta,
déjalo y anota en el commit para qué sirve.

> Nota para el ejecutor: `tinywasm/app` inyecta el store del proyecto (`h.DB`,
> respaldado por `.env`). Si lo quitas, ese lado deja de compilar — **cámbialo
> en el mismo plan sólo si eres tú quien lo dispara**; si no, limítate a la
> constante y sus escrituras y reporta el resto.

### Y que la decisión se lea en el log

El mensaje de `management.go:17` es correcto pero solo aparece en un sentido.
Registrar **siempre** la decisión, con su motivo, como constantes con nombre:

```
"Modo externo: encontrado %s"                  // cuando existe el archivo
"Modo interno: no existe %s — se sirve desde memoria"   // cuando no
```

La ruta completa, no solo el nombre: en el caso real que destapó esto el
handler buscaba `./web/main.go` (valores por defecto) cuando el archivo del
proyecto era `<raíz>/web/server.go`, y desde fuera era imposible saberlo.

## 4. Por qué este plan va DESPUÉS del de `tinywasm/app`

Hoy, en un sitio estático con el entregable versionado, el modo externo **no
llega a activarse** porque `app` se salta todo el cableado del servidor cuando
detecta ese conflicto (su plan lo explica y lo corrige). Ese accidente es lo
único que hoy impide que se vuelque el `client.wasm` de desarrollo (**1.9 MB**,
Go estándar) encima del entregable de release (**98 KB**, TinyGo).

Cuando `app` deje de escribir en el proyecto —lo primero de su plan— este ya no
es un riesgo. Antes, sí lo es.

**Este plan por sí solo no reactiva nada** (borra una clave muerta), pero
cualquier trabajo sobre el modo externo en este repo debe esperar a que `app`
esté publicado.

## 5. Tests — dónde van

Sigue la convención del repo (mira dónde están hoy los `*_test.go`; si hay
`tests/`, ahí).

| Test | Qué prueba |
|---|---|
| `TestModoExternoLoDecideElArchivo` | con `<AppRootDir>/<SourceDir>/<MainInputFile>` presente → estrategia externa |
| `TestModoInternoSinArchivo` | sin él → estrategia interna |
| `TestNoSeEscribeNingunaClaveDeModo` | un `Store` espía no recibe **ninguna** escritura durante `StartServer` |
| `TestLaDecisionSeRegistraConLaRutaCompleta` | el log contiene la ruta absoluta comprobada |

## 6. Criterios de aceptación

| # | Comprobación | Esperado |
|---|---|---|
| 1 | `go test ./...` | verde |
| 2 | `grep -rn "server_external_mode\|StoreKeyExternalServer" .` | vacío |
| 3 | Arranque con y sin archivo de servidor | el log dice cuál eligió y por qué, con la ruta |
| 4 | `.env` del proyecto tras arrancar | ya no aparece `server_external_mode` |

## 7. Etapas

| # | Etapa | Archivos |
|---|---|---|
| 1 | Borrar la constante y sus dos escrituras | `server.go`, `management.go` |
| 2 | Decidir sobre `Store`/`noopStore` (§3) y anotarlo | `server.go` |
| 3 | Log de la decisión con ruta completa, con constantes | `management.go` |
| 4 | Tests | según convención del repo |
