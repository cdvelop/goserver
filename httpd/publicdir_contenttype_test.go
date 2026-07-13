package httpd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicDirServesCorrectContentType reproduce un bug real: TODO archivo servido
// desde PublicDir llegaba como "text/plain", así que el navegador imprimía el
// index.html como texto dentro de un <pre> en vez de renderizarlo, y la hoja de
// estilos no se aplicaba nunca.
//
// Causa: el mux corre primero y no encuentra la ruta, así que http.NotFound escribe
// "Content-Type: text/plain" + "X-Content-Type-Options: nosniff" en el mapa de
// cabeceras y llama WriteHeader(404). statusRecorder suprime el ESTADO 404 para dar
// paso al fallback, pero las CABECERAS ya quedaron escritas. Después http.ServeFile
// respeta un Content-Type existente y no lo corrige.
//
// El fallback debe descartar las cabeceras que dejó el 404 abortado.
func TestPublicDirServesCorrectContentType(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<!doctype html><html><body>hola</body></html>")
	write("style.css", ":root{--x:1}")

	s := New(Config{PublicDir: dir})
	handler, err := s.Handler()
	if err != nil {
		t.Fatalf("Handler(): %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantType string
	}{
		{"index.html en la raiz", "/", "text/html"},
		{"hoja de estilos", "/style.css", "text/css"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			res := rec.Result()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d; se esperaba 200", res.StatusCode)
			}

			got := res.Header.Get("Content-Type")
			if !strings.HasPrefix(got, tt.wantType) {
				t.Errorf("Content-Type = %q; se esperaba %q.\n"+
					"Con text/plain el navegador imprime el HTML como texto en un <pre> "+
					"y no aplica el CSS: la página no renderiza.", got, tt.wantType)
			}
			if res.Header.Get("X-Content-Type-Options") == "nosniff" && strings.HasPrefix(got, "text/plain") {
				t.Error("la respuesta arrastra las cabeceras del 404 abortado")
			}
		})
	}
}
