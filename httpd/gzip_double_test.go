package httpd

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/router"
)

// wasmMagic: los 4 bytes con los que empieza todo binario WebAssembly.
// El navegador los exige en instantiateStreaming; si encuentra otra cosa, aborta.
var wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}

// TestGzipDoesNotDoubleEncode reproduce un bug real: el binario wasm llegaba al
// navegador comprimido DOS veces y la app no arrancaba nunca, con este error:
//
//	CompileError: WebAssembly.instantiateStreaming():
//	expected magic word 00 61 73 6d, found 1f 8b 08 00
//
// (1f 8b 08 00 es la firma de gzip: debajo de una capa había otra.)
//
// Causa: `tinywasm/client` comprime el wasm por su cuenta y declara
// Content-Encoding: gzip; la batería Gzip del servidor lo envolvía y volvía a
// comprimirlo encima. El navegador descomprime UNA capa y encuentra gzip.
//
// Comprimir es responsabilidad del transporte, y el transporte debe negarse a
// re-codificar una respuesta que ya viene codificada — venga del handler que venga.
func TestGzipDoesNotDoubleEncode(t *testing.T) {
	payload := append(wasmMagic, []byte("cuerpo del modulo wasm")...)

	// Un handler que ya comprime por su cuenta y lo declara (como hace client).
	selfCompressing := func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/wasm")
		ctx.SetHeader("Content-Encoding", "gzip")
		gz := gzip.NewWriter(ctx)
		defer gz.Close()
		gz.Write(payload)
	}

	h := Gzip(selfCompressing)

	req := httptest.NewRequest(http.MethodGet, "/client.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	h(&httpContext{w: rec, r: req})

	res := rec.Result()
	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q; se esperaba gzip", got)
	}

	// El navegador descomprime exactamente UNA capa. Hacemos lo mismo.
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("la respuesta no es gzip válido: %v", err)
	}
	defer zr.Close()

	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("leyendo la capa gzip: %v", err)
	}

	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		t.Fatal("DOBLE GZIP: tras descomprimir una capa aparece otra. " +
			"El navegador vería 1f 8b 08 00 en vez del magic word wasm y la app no arrancaría.")
	}
	if !bytes.Equal(body, payload) {
		t.Errorf("cuerpo corrupto tras una descompresión:\ngot:  %x\nwant: %x", body, payload)
	}
}

// La batería debe seguir comprimiendo lo que NO viene codificado.
func TestGzipStillCompressesPlainResponses(t *testing.T) {
	payload := []byte("body sin comprimir que sí debe viajar en gzip")

	plain := func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "text/plain")
		ctx.Write(payload)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	Gzip(plain)(&httpContext{w: rec, r: req})

	res := rec.Result()
	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q; una respuesta plana SÍ debe comprimirse", got)
	}

	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("la respuesta debería ser gzip válido: %v", err)
	}
	defer zr.Close()

	body, _ := io.ReadAll(zr)
	if !bytes.Equal(body, payload) {
		t.Errorf("cuerpo = %q; se esperaba %q", body, payload)
	}
}
