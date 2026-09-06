package httpd

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"webtyp.com/router"
)

// TestServerPipelineDoesNotDoubleEncode ejercita el Server COMPLETO, no el middleware
// aislado: mux → wrapWithBatteries → wrapWithGlobalBatteries. Es el camino real que
// recorre /client.wasm en producción, y donde el bug se veía de verdad:
//
//	CompileError: WebAssembly.instantiateStreaming():
//	expected magic word 00 61 73 6d, found 1f 8b 08 00
//
// El handler imita a webtyp/client: comprime el wasm él mismo y lo declara.
// La batería Gzip del servidor no debe volver a comprimirlo encima.
func TestServerPipelineDoesNotDoubleEncode(t *testing.T) {
	wasm := append([]byte{0x00, 0x61, 0x73, 0x6d}, []byte("cuerpo del modulo")...)

	s := New(Config{Gzip: true})
	s.Router().PublicAsset("/client.wasm", func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/wasm")
		ctx.SetHeader("Content-Encoding", "gzip")
		gz, _ := gzip.NewWriterLevel(ctx, gzip.BestCompression)
		gz.Write(wasm)
		gz.Close()
	})

	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler(): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/client.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; se esperaba 200", res.StatusCode)
	}

	// El navegador descomprime exactamente UNA capa.
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
		t.Fatal("DOBLE GZIP en el pipeline del Server: bajo una capa hay otra. " +
			"El navegador vería 1f 8b 08 00 en vez del magic word wasm y la app no arrancaría.")
	}
	if !bytes.Equal(body, wasm) {
		t.Errorf("cuerpo corrupto:\ngot:  %x\nwant: %x", body, wasm)
	}
}
