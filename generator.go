package goserver

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/cdvelop/mdgo"
)

//go:embed templates/*
var embeddedFS embed.FS

type serverTemplateData struct {
	AppPort   string
	PublicDir string
}

// generateServerFromEmbeddedMarkdown creates the external server go file from the embedded markdown
// It never overwrites an existing file. If template processing fails, logs to Logger and uses raw markdown.
func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
	// The new convention places the generated main.go file in the SourceDir
	targetPath := path.Join(h.AppRootDir, h.SourceDir, h.mainFileExternalServer)

	// Never overwrite existing files
	if _, err := os.Stat(targetPath); err == nil {
		if h.Logger != nil {
			h.Logger("Server file already exists at", targetPath, ", skipping generation")
		}
		return nil
	}

	data := serverTemplateData{
		AppPort:   h.AppPort,
		PublicDir: h.PublicDir,
	}

	// read embedded markdown
	raw, errRead := embeddedFS.ReadFile("templates/server_basic.md")
	embeddedContent := ""
	if errRead == nil {
		embeddedContent = string(raw)
	} else {
		// fallback to empty
		embeddedContent = ""
	}

	processed, err := h.processTemplate(embeddedContent, data)
	if err != nil {
		// processTemplate already logs; fallback to embedded raw content
		processed = embeddedContent
	}

	// Use mdgo to extract Go code from markdown
	writer := func(name string, data []byte) error {
		if err := os.MkdirAll(path.Dir(name), 0o755); err != nil {
			return err
		}
		return os.WriteFile(name, data, 0o644)
	}

	// mdgo needs the full destination path
	destDir := path.Join(h.AppRootDir, h.SourceDir)
	m := mdgo.New(h.AppRootDir, destDir, writer).
		InputByte([]byte(processed))

	if h.Logger != nil {
		m.SetLogger(h.Logger)
	}

	// Extract to the main file name (mdgo will handle the path joining)
	if err := m.Extract(h.mainFileExternalServer); err != nil {
		return fmt.Errorf("extracting go code from markdown: %w", err)
	}

	if h.Logger != nil {
		h.Logger("Generated server file at", targetPath)
	}
	return nil
}

func (h *ServerHandler) processTemplate(markdown string, data serverTemplateData) (string, error) {
	tmpl, err := template.New("server").Parse(markdown)
	if err != nil {
		if h.Logger != nil {
			h.Logger("Template parsing error (using fallback):", err)
		}
		return markdown, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		if h.Logger != nil {
			h.Logger("Template execution error (using fallback):", err)
		}
		return markdown, err
	}
	return buf.String(), nil
}
