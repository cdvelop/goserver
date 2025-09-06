package goserver

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*
var embeddedFS embed.FS

type serverTemplateData struct {
	AppPort      string
	PublicFolder string
	RootFolder   string
}

// generateServerFromEmbeddedMarkdown creates the external server go file from the embedded markdown
// It never overwrites an existing file. If template processing fails, logs to Logger and uses raw markdown.
func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
	targetPath := path.Join(h.RootFolder, h.mainFileExternalServer)

	// Never overwrite existing files
	if _, err := os.Stat(targetPath); err == nil {
		if h.Logger != nil {
			h.Logger("Server file already exists at", targetPath, ", skipping generation")
		}
		return nil
	}

	data := serverTemplateData{
		AppPort:      h.AppPort,
		PublicFolder: h.PublicFolder,
		RootFolder:   h.RootFolder,
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

	code := h.extractGoCodeFromMarkdown(processed)
	if code == "" {
		return fmt.Errorf("no go code blocks found in embedded server definition")
	}

	// Ensure target directory exists
	if err := os.MkdirAll(h.RootFolder, 0755); err != nil {
		return fmt.Errorf("creating root folder: %w", err)
	}

	if err := os.WriteFile(targetPath, []byte(code), 0644); err != nil {
		return fmt.Errorf("writing server file: %w", err)
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

func (h *ServerHandler) extractGoCodeFromMarkdown(markdown string) string {
	// pattern to capture ```go ... ``` blocks, DOTALL mode
	pattern := "(?s)```go\\n(.*?)```"
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(markdown, -1)
	var blocks []string
	for _, m := range matches {
		if len(m) > 1 {
			blocks = append(blocks, strings.TrimSpace(m[1]))
		}
	}
	return strings.Join(blocks, "\n\n")
}
