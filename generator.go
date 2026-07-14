package server

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/tinywasm/markdown"
)

//go:embed templates/*
var embeddedFS embed.FS

type serverTemplateData struct {
	AppPort   string
	PublicDir string
}

// getExpectedServerContent returns what the template would generate
// Used for comparison to detect if user modified the server file
func (h *ServerHandler) getExpectedServerContent() (string, error) {
	data := serverTemplateData{
		AppPort:   h.Port(),
		PublicDir: h.PublicDir,
	}

	// read embedded markdown
	raw, errRead := embeddedFS.ReadFile("templates/server_basic.md")
	if errRead != nil {
		return "", fmt.Errorf("reading embedded template: %w", errRead)
	}

	processed, err := h.processTemplate(string(raw), data)
	if err != nil {
		return "", err
	}

	var extractedCode string
	captureWriter := func(name string, data []byte) error {
		extractedCode = string(data)
		return nil
	}

	// use a temporary markdown Doc to extract code from the processed template
	// use "." as dummy destination as captureWriter handles the content
	m := markdown.New("", ".", captureWriter).
		InputByte([]byte(processed))

	if err := m.Extract(h.mainFileExternalServer); err != nil {
		return "", fmt.Errorf("extracting go code from template: %w", err)
	}

	return extractedCode, nil
}

// generateServerFromEmbeddedMarkdown creates the external server go file from the embedded markdown
// It never overwrites an existing file. If template processing fails, logs to Logger and uses raw markdown.
func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
	// The new convention places the generated main.go file in the SourceDir
	targetPath := path.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)

	// Never overwrite existing files
	if _, err := os.Stat(targetPath); err == nil {
		h.log("Server file already exists at", targetPath, ", skipping generation")
		return nil
	}

	expectedContent, err := h.getExpectedServerContent()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(path.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("creating server directory: %w", err)
	}

	if err := os.WriteFile(targetPath, []byte(expectedContent), 0o644); err != nil {
		return fmt.Errorf("writing server file: %w", err)
	}

	h.log("Generated server file at", targetPath)
	return nil
}

func (h *ServerHandler) processTemplate(markdown string, data serverTemplateData) (string, error) {
	tmpl, err := template.New("server").Parse(markdown)
	if err != nil {
		h.log("Template parsing error (using fallback):", err)
		return markdown, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		h.log("Template execution error (using fallback):", err)
		return markdown, err
	}
	return buf.String(), nil
}
