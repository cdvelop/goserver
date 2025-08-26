# GoServer Auto-Generation Refactor Plan

## Overview

This document outlines the refactoring plan to transform GoServer from a dual-mode server (interna### Phase 2: Template Processing and Code Extraction

#### 2.1 Add Template Processing Method
```go
// Template data structure for passing values to the markdown template
type serverTemplateData struct {
    AppPort      string
    PublicFolder string
    RootFolder   string
}

func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
    // 1. Load embedded markdown file
    // 2. Process markdown as Go template with Config values
    // 3. Parse processed markdown and extract Go code blocks using regex
    // 4. Concatenate all Go code blocks into single file content
    // 5. Write final Go code to mainFileExternalServer path
    // 6. Log generation to Writer
}
```

#### 2.2 Template Processing and Code Extraction Logic
1. Create template data structure with Config values
2. Process embedded markdown using `html/template`
3. Use regex to find all code blocks: ```` ```go\n(.*?)\n``` ````
4. Extract and concatenate all Go code blocks
5. Write complete Go file to target location

#### 2.3 Implementation Structure
```go
//go:embed templates/server_definition.md
var embeddedServerDefinition string

func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
    // Process template
    templateData := serverTemplateData{
        AppPort:      h.AppPort,
        PublicFolder: h.PublicFolder,
        RootFolder:   h.RootFolder,
    }
    
    processedMarkdown := h.processTemplate(embeddedServerDefinition, templateData)
    content := h.extractGoCodeFromMarkdown(processedMarkdown)
    return h.writeServerFile(content)
}
```
tional external server) to an auto-generating external server approach. The new implementation will automatically create a basic server template when the external server file doesn't exist, eliminating the need for the internal server and ensuring developers always work with a customizable external server.

## Current Architecture Analysis

### Current Behavior
1. **Dual Server Mode**: GoServer currently operates in two modes:
   - **Internal Server**: Serves static files using `http.FileServer` when no external server exists
   - **External Server**: Compiles and runs custom Go server when `main.server.go` exists

2. **Decision Logic** (in `Start.go`):
   ```go
   if _, err := os.Stat(path.Join(h.RootFolder, h.mainFileExternalServer)); os.IsNotExist(err) {
       h.StartInternalServerFiles()  // Static file server
   } else {
       h.startServer()       // Custom Go server
   }
   ```

3. **Current Dependencies**:
   - `github.com/cdvelop/gobuild` - Go compilation management
   - `github.com/cdvelop/gorun` - External process execution
   - Standard `net/http` - Internal file server

### Current Public API
```go
// Core Types
type Config struct { ... }
type ServerHandler struct { ... }

// Constructor
func New(c *Config) *ServerHandler

// Public Methods
func (h *ServerHandler) Start(wg *sync.WaitGroup)
func (h *ServerHandler) startServer() error
func (h *ServerHandler) StartInternalServerFiles()
func (h *ServerHandler) StopInternalServer() error
func (h *ServerHandler) RestartInternalServer() error
func (h *ServerHandler) RestartServer() error
func (h *ServerHandler) RestartServer() error
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error
func (h *ServerHandler) MainFilePath() string
func (h *ServerHandler) Name() string
func (h *ServerHandler) UnobservedFiles() []string
```

## Proposed Architecture

### New Behavior
1. **Single Server Mode**: Always use external server approach
2. **Auto-Generation**: If external server file doesn't exist, create it from template
3. **Template-Based**: Use embedded Markdown template with Go code blocks
4. **Simplified Logic**: Remove internal server complexity

### New Decision Logic
```go
// In Start()
if _, err := os.Stat(path.Join(h.RootFolder, h.mainFileExternalServer)); os.IsNotExist(err) {
    err := h.generateServerFromEmbeddedMarkdown()  // NEW: Extract Go code from embedded markdown
    if err != nil {
        return fmt.Errorf("failed to generate server from embedded markdown: %w", err)
    }
}
// Always start external server
err := h.startServer()
```

## Implementation Plan

### Phase 1: Embedded Markdown Server Definition

#### 1.1 Create Server Definition
- **Location**: `templates/server_definition.md` (embedded in binary)
- **Format**: Markdown with complete Go code blocks
- **Content**: Complete HTTP server equivalent to current internal server functionality
- **Logic**: Extract Go code blocks and write directly to file (no template processing)

**Server Definition Structure**:
```markdown
# Default GoServer Implementation

This is the default server implementation that gets created when no external server exists.
You can modify this server according to your needs.

## Server Code

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "{{.AppPort}}"  // Template variable
    }
    
    publicDir := "{{.PublicFolder}}"  // Template variable
    
    // Serve static files
    fs := http.FileServer(http.Dir(publicDir))
    http.Handle("/", fs)
    
    // Health check endpoint
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, "Server is running")
    })
    
    fmt.Printf("Server starting on port %s\n", port)
    fmt.Printf("Serving static files from: %s\n", publicDir)
    
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}
```

## Customization

You can modify this server to add:
- Custom routes and handlers
- Middleware for authentication, logging, etc.
- API endpoints for your application
- Database connections
- WebSocket support

## Examples

### Adding a custom route:
```go
http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprintln(w, `{"message": "Hello from custom API!"}`)
})
```


#### 1.2 Template Processing System
- **Template Engine**: Go's `html/template` for processing the markdown
- **Extraction**: Find all ```go code blocks and combine them using regex
- **Template Variables**: Use Go template syntax `{{.AppPort}}`, `{{.PublicFolder}}`, `{{.RootFolder}}`
- **Embed**: Use `//go:embed` to include markdown file in binary

### Phase 2: Code Generation

#### 2.1 Add Template Generation Method
```go
func (h *ServerHandler) generateServerFromTemplate() error {
    // 1. Load embedded template
    // 2. Process template with Config variables
    // 3. Extract Go code blocks from processed Markdown
    // 4. Write Go code to mainFileExternalServer path
    // 5. Log generation to Writer
}
```

#### 2.2 Template Processing Logic
1. Parse Markdown template
2. Substitute `{{.AppPort}}` and `{{.PublicFolder}}` placeholders
3. Extract code blocks marked as `go`
4. Combine code blocks into single Go file
5. Write to target location

### Phase 3: API Cleanup

#### 3.1 Remove Internal Server Methods
**Methods to Remove**:
- `StartInternalServerFiles()`
- `StopInternalServer()`
- `RestartInternalServer()`
- `RestartServer()` (simplify to only handle external server)

#### 3.2 Simplify ServerHandler Struct
**Remove Fields**:
- `internalServerRun bool`
- `server *http.Server`

**Updated ServerHandler**:
```go
type ServerHandler struct {
    *Config
    mainFileExternalServer string // eg: main.server.go
    goCompiler             *gobuild.GoBuild
    goRun                  *gorun.GoRun
}
```

#### 3.3 New Public API
```go
// Maintained Public API
func New(c *Config) *ServerHandler
func (h *ServerHandler) Start(wg *sync.WaitGroup) => StartServer
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error
func (h *ServerHandler) MainFilePath() string
func (h *ServerHandler) Name() string
func (h *ServerHandler) UnobservedFiles() []string


// methods (implementation details)
func (h *ServerHandler) startServer() error => StartServer
func (h *ServerHandler) RestartServer() error => RestartServer
func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error
func (h *ServerHandler) processTemplate(markdown string, data serverTemplateData) string
func (h *ServerHandler) extractGoCodeFromMarkdown(markdown string) string
func (h *ServerHandler) writeServerFile(content string) error
```

### Phase 4: File Event Handling

#### 4.1 Simplify NewFileEvent Logic
- Remove internal server event handling
- Focus only on external server file changes
- Maintain hot-reload functionality for external server

#### 4.2 Updated Event Logic
```go
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error {
    switch event {
    case "write":
        if fileName == h.mainFileExternalServer || extension == ".go" {
            return h.RestartServer()
        }
    case "create":
        if fileName == h.mainFileExternalServer {
            return h.startServer()
        }
    }
    return nil
}
```

## API Changes

### Config Struct Updates
**Current Config will be updated for better clarity:**

```go
type Config struct {
    RootFolder                  string          // eg: web
    MainFileWithoutExtension    string          // eg: main.server
    ArgumentsForCompilingServer func() []string // eg: []string{"-X 'main.version=v1.0.0'"}
    ArgumentsToRunServer        func() []string // eg: []string{"dev" }
    PublicFolder                string          // eg: public
    AppPort                     string          // eg : 8080
    Logger                      io.Writer       // For logging output (renamed from Writer)
    ExitChan                    chan bool       // Canal global para señalizar el cierre
}
```

**Change**: `Writer` field renamed to `Logger` for better semantic clarity, as it's primarily used for logging operations.

## Breaking Changes

### Removed Public Methods
- `StartInternalServerFiles()`
- `StopInternalServer()`
- `RestartInternalServer()`

### Behavioral Changes
- No more internal static file server
- External server file is always created if missing
- Simplified server management (single mode only)

## Migration Guide

### For Library Users
1. **No Code Changes Required**: Public API remains compatible for core functionality
2. **Behavior Change**: Internal server mode no longer exists
3. **New Feature**: Auto-generated server template when starting new projects

### For Existing Projects
1. **With Existing External Server**: No changes needed, continues working
2. **Using Internal Server Only**: Will automatically get generated external server template
3. **Custom Logic**: Review any code depending on removed methods

## Implementation Questions & Alternatives

### Question 1: Template Data Structure Extension
**Current Decision**: Keep `serverTemplateData` minimal with only essential fields

**Final Structure**:
```go
type serverTemplateData struct {
    AppPort      string // e.g., "8080"
    PublicFolder string // e.g., "public"
    RootFolder   string // e.g., "web"
}
```

**Extension Decision**: **NO** - Start minimal, do not add additional fields like `MainFileWithoutExtension` or `ArgumentsToRunServer` unless specifically needed.

### Question 2: Template Processing Error Handling
**Current Decision**: Log error using Writer and continue with fallback

**What happens when template processing fails?**

**Options**:
- **A.** Fail completely with template error details
- **B.** Fall back to unprocessed markdown (no variable substitution)
- **C.** Use default values for failed template variables

**Final Decision**: **Log error and continue** - Use the Config's Writer (logger) to report template errors, but don't fail completely

**Implementation**:
```go
func (h *ServerHandler) processTemplate(markdown string, data serverTemplateData) string {
    tmpl, err := template.New("server").Parse(markdown)
    if err != nil {
        fmt.Fprintf(h.Writer, "Template parsing error (using fallback): %v\n", err)
        return markdown // Use original markdown without processing
    }
    
    var buf bytes.Buffer
    err = tmpl.Execute(&buf, data)
    if err != nil {
        fmt.Fprintf(h.Writer, "Template execution error (using fallback): %v\n", err)
        return markdown // Use original markdown without processing
    }
    
    return buf.String()
}
```

**Note**: The Config.Writer field should be renamed to Logger for better clarity, as it's primarily used for logging operations.

### Question 3: Template Variable Validation
**Current Decision**: NO validation - Keep implementation simple

**Should we validate template variables before processing?**

**Options**:
- **A.** Validate that all required fields are non-empty
- **B.** Validate format (e.g., port is numeric)
- **C.** No validation, let template processing handle errors

**Final Decision**: **C** (no validation) - Template processing will handle any issues, and errors will be logged using the Writer/Logger.

**Rationale**: Simpler implementation, fewer edge cases to handle, and template errors are already handled gracefully.
### Question 4: Regex Pattern for Code Extraction
**Regex pattern for extracting Go code blocks:**

**Current Decision**: Use regex for controlled markdown content

**Final Pattern**:
```go
pattern := `(?s)` + "`" + `{3}go\n(.*?)` + "`" + `{3}`
// (?s) enables . to match newlines
// {3} matches exactly three backticks
// (.*?) non-greedy capture of code content
```

**Implementation confirmed**: Simple regex approach works well for controlled content.

### Question 5: Multiple Code Blocks Concatenation
**How to handle multiple ```go blocks in the markdown:**

**Current Decision**: **Concatenate all blocks** - confirmed approach

**Implementation Strategy**:
```go
func (h *ServerHandler) extractGoCodeFromMarkdown(markdown string) string {
    pattern := `(?s)` + "`" + `{3}go\n(.*?)` + "`" + `{3}`
    re := regexp.MustCompile(pattern)
    matches := re.FindAllStringSubmatch(markdown, -1)
    
    var codeBlocks []string
    for _, match := range matches {
        if len(match) > 1 {
            codeBlocks = append(codeBlocks, strings.TrimSpace(match[1]))
        }
    }
    
    return strings.Join(codeBlocks, "\n\n")
}
```

**Benefits**: 
- Allows modular organization in markdown
- Simple concatenation works for most cases
- Developer can see the structure in markdown

### Question 6: File Overwrite Protection Implementation
**Current Decision**: NEVER overwrite existing files - confirmed

**Implementation**:
```go
func (h *ServerHandler) generateServerFromEmbeddedMarkdown() error {
    targetPath := path.Join(h.RootFolder, h.mainFileExternalServer)
    
    // CRITICAL: Never overwrite existing files
    if _, err := os.Stat(targetPath); err == nil {
        fmt.Fprintf(h.Writer, "Server file already exists at %s, skipping generation\n", targetPath)
        return nil // Not an error, just skip generation
    }
    
    // Proceed with generation...
}
```

**Edge Cases Handled**:
- **Empty files**: Still don't overwrite (preserve user intent)
- **Different permissions**: Respect existing permissions
- **Directory doesn't exist**: Create directory if needed

**Final Policy**: Never overwrite ANY existing file, regardless of content or state.

---

## Final Implementation Decisions Summary

### ✅ Confirmed Decisions:
1. **Template Data Structure**: Minimal with only `AppPort`, `PublicFolder`, `RootFolder`
2. **Error Handling**: Log errors using Writer/Logger, continue with fallback (don't fail completely)
3. **Validation**: No validation of template variables
4. **Template Versioning**: Not relevant for this implementation
5. **File Protection**: Never overwrite existing files under any circumstances

### 🔄 Implementation Changes:
- Config.Writer → Config.Logger (field rename for clarity)
- Template processing with graceful error handling and logging
- Regex-based code extraction with concatenation of multiple blocks
- Always check file existence before generation

---

## Testing Strategy

### Unit Tests
1. **Template Processing**: Test Go template processing with serverTemplateData
2. **Markdown Parsing**: Test Go code block extraction using regex (concatenation of multiple blocks)
3. **File Generation**: Test file creation and content accuracy
4. **Error Handling**: Test various failure scenarios

### Integration Tests
1. **Full Workflow**: Test complete generation → compilation → execution cycle
2. **Hot Reload**: Test file change detection and restart
3. **Multiple Projects**: Test in different directory structures

### Backward Compatibility Tests
1. **Existing External Servers**: Ensure no regression
2. **API Compatibility**: Test remaining public methods
3. **Migration Scenarios**: Test various upgrade paths

## Timeline & Milestones

### Milestone 1: Template Processing System (Week 1)
- [ ] Create server definition markdown file with template variables
- [ ] Implement Go template processing with serverTemplateData and graceful error handling
- [ ] Implement regex-based Go code extraction (concatenating multiple blocks)
- [ ] Add embedded markdown to binary
- [ ] Unit tests for template and parsing system

### Milestone 2: Code Generation (Week 2)
- [ ] Implement `generateServerFromEmbeddedMarkdown()` with file overwrite protection
- [ ] Add serverTemplateData structure and processing
- [ ] Integrate with Start() method
- [ ] Add error logging using Logger field
- [ ] Integration tests

### Milestone 3: API Cleanup (Week 3)
- [ ] Remove internal server methods
- [ ] Clean up ServerHandler struct (remove internal server fields)
- [ ] Update Config.Writer to Config.Logger
- [ ] Update NewFileEvent logic
- [ ] Update documentation

### Milestone 4: Testing & Polish (Week 4)
- [ ] Comprehensive test suite
- [ ] Performance testing
- [ ] Documentation updates
- [ ] Migration guide

## Risk Assessment

### High Risk
- **Breaking Changes**: Removed public methods may break existing code
- **Markdown Parsing Bugs**: Errors in code extraction could break all new projects

### Medium Risk
- **Performance**: Markdown parsing adds startup overhead
- **Complexity**: New parsing system adds maintenance burden

### Low Risk
- **Compatibility**: Core functionality remains the same
- **User Experience**: Auto-generation improves developer experience

## Success Criteria

1. **Functionality**: All existing external server projects continue working
2. **Developer Experience**: New projects get working server with zero setup
3. **Maintainability**: Cleaner codebase with single server mode
4. **Performance**: No significant performance regression
5. **Documentation**: Clear migration path and usage examples

---

**Status**: Ready for Implementation
**Last Updated**: August 14, 2025
**Author**: GoServer Refactoring Team

## Implementation Summary

This refactoring plan transforms GoServer from a dual-mode system to a streamlined auto-generating external server approach. Key decisions finalized:

- **Template System**: Go html/template with minimal serverTemplateData structure
- **Error Handling**: Graceful fallback with logging via Config.Logger (renamed from Writer)
- **Code Extraction**: Regex-based parsing with concatenation of multiple Go code blocks
- **File Protection**: Never overwrite existing files under any circumstances
- **API Simplification**: Remove internal server complexity, maintain only essential public methods

The implementation maintains backward compatibility for existing external servers while providing automatic server generation for new projects.
