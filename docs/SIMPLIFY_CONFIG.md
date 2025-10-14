# Config Simplification Plan

## Objective
Simplify the `goserver.Config` structure to align with modern conventions used in `tinywasm` and `goflare` packages, reducing complexity and improving clarity.

## Current State Analysis

### Current Config Structure
```go
type Config struct {
    AppRootDir                  string               // e.g., /home/user/project (application root directory)
    RootFolder                  string               // e.g., web (relative to AppRootDir or absolute path)
    MainFileWithoutExtension    string               // e.g., main.server
    ArgumentsForCompilingServer func() []string      // e.g., []string{"-X 'main.version=v1.0.0'"}
    ArgumentsToRunServer        func() []string      // e.g., []string{"dev"}
    PublicFolder                string               // e.g., public
    AppPort                     string               // e.g., 8080
    Logger                      func(message ...any) // For logging output
    ExitChan                    chan bool            // Global channel to signal shutdown
}
```

### Problems Identified
1. **Ambiguous naming**: `RootFolder` is confusing - it's actually the source directory location
2. **Implicit main file name**: `MainFileWithoutExtension` requires adding `.go` programmatically
3. **Unclear compilation flow**: Not clear where compilation outputs go vs where source files live
4. **Inconsistent with sister packages**: `tinywasm` and `goflare` use clearer `SourceDir`/`OutputDir` pattern
5. **PublicFolder redundancy**: This is really just metadata about where public assets are, not directly used by goserver's core function

### Current Usage Pattern (from golite)
```go
serverHandler = goserver.New(&goserver.Config{
    AppRootDir:                  h.rootDir,                                          // "/home/user/project"
    RootFolder:                  filepath.Join(h.rootDir, h.config.CmdAppServerDir()), // "/home/user/project/src/cmd/appserver"
    MainFileWithoutExtension:    "main",
    ArgumentsForCompilingServer: nil,
    ArgumentsToRunServer:        nil,
    PublicFolder:                h.config.WebPublicDir(),                            // "src/web/public"
    AppPort:                     h.config.ServerPort(),                              // "8080"
    Logger:                      serverLogger,
    ExitChan:                    h.exitChan,
})
```

**Key observation**: `RootFolder` is actually the **full path to source directory** (not a relative path to a "root folder").

## Proposed Target Structure

### New Simplified Config
```go
type Config struct {
    AppRootDir                  string               // e.g., /home/user/project (application root directory)
    SourceDir                   string               // directory location of main.go e.g., src/cmd/appserver (relative to AppRootDir)
    OutputDir                   string               // compilation and execution directory e.g., deploy/appserver (relative to AppRootDir)
    ArgumentsForCompilingServer func() []string      // e.g., []string{"-X 'main.version=v1.0.0'"}
    ArgumentsToRunServer        func() []string      // e.g., []string{"dev"}
    AppPort                     string               // e.g., 8080
    Logger                      func(message ...any) // For logging output
    ExitChan                    chan bool            // Global channel to signal shutdown
}
```

### Changes Summary
- ✅ **Remove**: `RootFolder` → Replace with `SourceDir` (clearer semantics)
- ✅ **Remove**: `MainFileWithoutExtension` → Use convention: always "main.go"
- ✅ **Remove**: `PublicFolder` → Not core to goserver's function (can be derived or passed via arguments if needed)
- ✅ **Add**: `OutputDir` → Explicit output location for compiled binary and execution

### Benefits
1. **Clearer semantics**: `SourceDir` clearly indicates where source code lives
2. **Convention over configuration**: Main file is always `main.go` (Go standard)
3. **Separation of concerns**: Clear distinction between source location and output location
4. **Consistency**: Matches `tinywasm` and `goflare` patterns
5. **Reduced cognitive load**: Fewer fields to understand and configure

## Implementation Decisions ✅

All decisions have been confirmed and approved for implementation.

### ✅ D1: Remove `MainFileWithoutExtension` - Enforce `main.go` Convention
**Decision**: **APPROVED** - Remove completely, enforce `main.go` convention

**Rationale**: 
- Go convention is `main.go`
- Simplifies implementation
- Reduces configuration complexity
- Sister packages (`tinywasm`, `goflare`) use similar conventions
- If users need different entry points, they can structure their source directory accordingly

---

### ✅ D2: OutputDir Usage - Compile and Execute from OutputDir
**Decision**: **APPROVED** - Compile binary to `OutputDir`, execute from `OutputDir`

**Rationale**:
- Clean separation between source and build artifacts
- Follows modern build tool patterns (dist/, build/, deploy/)
- Easier cleanup (just delete OutputDir)
- Better for deployment scenarios

**Implementation details**:
- Input file: `AppRootDir/SourceDir/main.go`
- Output binary: `AppRootDir/OutputDir/main` (or `main.exe` on Windows)
- Working directory for execution: `AppRootDir/OutputDir`

---

### ✅ D3: Remove PublicFolder - Not Needed
**Decision**: **APPROVED** - Remove completely

**Rationale**:
- `PublicFolder` is not useful because if `SourceDir` is not declared, the generated template is saved there
- Not used by core `goserver` functionality
- Not used by `gobuild` integration
- Can be passed via `ArgumentsToRunServer` if the generated server template needs it at runtime
- Simplifies configuration

**Note**: The generated server template location is determined by `SourceDir`, not `PublicFolder`

---

### ✅ D4: SourceDir and OutputDir Relative to AppRootDir
**Decision**: **APPROVED** - Both paths are relative to `AppRootDir`

**Example**:
```go
SourceDir: "src/cmd/appserver"  // relative to AppRootDir
OutputDir: "deploy/appserver"   // relative to AppRootDir
```

**Rationale**:
- Consistency across packages (matches `tinywasm` and `goflare`)
- More portable (no hard-coded absolute paths)
- Easier to reason about project structure
- Cleaner configuration

---

### ✅ D5: Breaking Change - No Backward Compatibility
**Decision**: **APPROVED** - Breaking change only (no compatibility layer)

**Rationale**:
- Clean break, no technical debt
- Package is early stage (internal usage primarily)
- Clear migration path is straightforward
- Better long-term maintainability
- Simpler implementation and maintenance

**Migration guide**:
```go
// OLD
Config{
    AppRootDir:               rootDir,
    RootFolder:               filepath.Join(rootDir, "src/cmd/appserver"),
    MainFileWithoutExtension: "main",
    PublicFolder:             "src/web/public",
    AppPort:                  "8080",
    Logger:                   logger,
    ExitChan:                 exitChan,
}

// NEW
Config{
    AppRootDir: rootDir,
    SourceDir:  "src/cmd/appserver",
    OutputDir:  "deploy/appserver",
    AppPort:    "8080",
    Logger:     logger,
    ExitChan:   exitChan,
}
```

---

### ✅ D6: Provide NewConfig() Helper Function
**Decision**: **APPROVED** - Provide `NewConfig()` helper

**Implementation**:
```go
func NewConfig() *Config {
    return &Config{
        AppRootDir: ".",
        SourceDir:  "src/cmd/appserver",
        OutputDir:  "deploy/appserver",
        AppPort:    "8080",
        Logger: func(message ...any) {
            // Silent by default
        },
        ExitChan: make(chan bool),
    }
}
```

**Rationale**:
- Matches `tinywasm.NewConfig()` pattern
- Provides sensible defaults
- Easier for new users
- Reduces boilerplate code

---

## Implementation Plan

### Phase 1: Preparation ✅ COMPLETED
1. ✅ Review server template usage of `PublicFolder` - **CONFIRMED**: Not used, template saves to `SourceDir`
2. ✅ Review all test files for current Config usage patterns - **DOCUMENTED**: See Testing Requirements
3. ✅ Document current `gobuild` integration requirements - **DOCUMENTED**: See Implementation Details
4. ✅ All decisions approved - **READY TO IMPLEMENT**

### Phase 2: Core Changes (goserver package)
1. **Update `Config` struct** in `goserver.go`:
   ```go
   type Config struct {
       AppRootDir                  string               // e.g., /home/user/project
       SourceDir                   string               // e.g., src/cmd/appserver (relative to AppRootDir)
       OutputDir                   string               // e.g., deploy/appserver (relative to AppRootDir)
       ArgumentsForCompilingServer func() []string
       ArgumentsToRunServer        func() []string
       AppPort                     string
       Logger                      func(message ...any)
       ExitChan                    chan bool
   }
   ```

2. **Add `NewConfig()` helper function**:
   ```go
   func NewConfig() *Config {
       return &Config{
           AppRootDir: ".",
           SourceDir:  "src/cmd/appserver",
           OutputDir:  "deploy/appserver",
           AppPort:    "8080",
           Logger:     func(message ...any) {},
           ExitChan:   make(chan bool),
       }
   }
   ```

3. **Update `New()` constructor**:
   - Remove `mainFileExternalServer` field (always use "main.go")
   - Update `gobuild.Config` mapping:
     ```go
     MainInputFileRelativePath: filepath.Join(c.SourceDir, "main.go")
     OutFolderRelativePath:     c.OutputDir
     OutName:                   "main"  // Fixed name
     ```
   - Update `goRun.Config`:
     ```go
     WorkingDir: filepath.Join(c.AppRootDir, c.OutputDir)
     ```

4. **Update `MainInputFileRelativePath()` method**:
   - Return `filepath.Join(c.SourceDir, "main.go")`
   - Remove complex absolute/relative path logic
   - Simplify to always use relative paths from `AppRootDir`

5. **Update server generation logic** (if applicable):
   - Ensure generated template is saved to `SourceDir/main.go`
   - Remove any `PublicFolder` references

### Phase 3: Update Dependents
1. **Update all test files** in `goserver` package:
   - `compilation_test.go` (3 tests)
   - `generator_test.go` (all tests)
   - `restart_cleanup_test.go`
   - `port_conflict_test.go`
   - `startserver_blackbox_test.go`
   - `startserver_integration_test.go`
   - `external_server_integration_test.go`

2. **Update `golite` package** integration:
   - File: `golite/section-build.go`
   - Change from:
     ```go
     RootFolder:               filepath.Join(h.rootDir, h.config.CmdAppServerDir()),
     MainFileWithoutExtension: "main",
     PublicFolder:             h.config.WebPublicDir(),
     ```
   - To:
     ```go
     SourceDir: h.config.CmdAppServerDir(),  // "src/cmd/appserver"
     OutputDir: "deploy/appserver",
     ```

3. **Update documentation**:
   - `goserver/README.md` - Update all Config examples
   - `goserver/docs/REFACTOR_AUTO_SERVER.md` - Update Config references
   - Add migration guide section to README

### Phase 4: Validation
1. Run `go test ./...` in `goserver` package
2. Run `go test ./...` in `golite` package
3. Manual integration test:
   - Start golite in example project
   - Verify server compiles to `deploy/appserver/`
   - Verify server executes correctly
   - Verify hot reload works
4. Test edge cases:
   - Empty Config values
   - Non-existent SourceDir
   - Non-existent OutputDir (should be created)
   - Windows compatibility

### Phase 5: Documentation
1. **Update README.md**:
   - New Config structure example
   - Add migration guide section
   - Update all code examples
   
2. **Create CHANGELOG.md entry**:
   ```markdown
   ## [Breaking] Config Simplification
   
   ### Changed
   - `RootFolder` → `SourceDir` (now relative to AppRootDir)
   - Removed `MainFileWithoutExtension` (always uses "main.go")
   - Removed `PublicFolder` (not needed)
   - Added `OutputDir` for explicit output location
   
   ### Migration Guide
   [Include migration examples]
   ```

3. **Update inline documentation**:
   - Add clear comments to each Config field
   - Document default values
   - Document path resolution behavior

---

## Testing Requirements

### Unit Tests to Update
- `TestStartServerAlwaysRecompiles` - Update Config initialization
- `TestNewFileEventTriggersRecompilation` - Update Config initialization
- `TestNewFileEventOnOtherGoFiles` - Update Config initialization
- `TestStartServerGeneratesExternalFile` - Update Config and assertions
- `TestRestartCleanup` - Update Config initialization
- `TestPortConflictCleanup` - Update Config initialization
- All tests in `generator_test.go`
- All tests in `compilation_test.go`

### Integration Tests to Verify
- Server compilation with new OutputDir
- Server execution from OutputDir
- File watching and hot reload
- Template generation compatibility

### Edge Cases to Test
1. Empty/nil Config values
2. Absolute vs relative path handling
3. Windows path compatibility
4. OutputDir doesn't exist (should be created)
5. SourceDir doesn't exist (should error clearly)

---

## Risk Assessment

### Breaking Changes Impact
- **Severity**: HIGH - All users must update their code
- **Scope**: Internal packages only (golite primarily)
- **Mitigation**: Clear migration guide + early communication

### Technical Risks
1. **Path resolution changes**: New SourceDir/OutputDir logic might break edge cases
   - Mitigation: Comprehensive path testing
2. **gobuild integration**: Mapping changes might affect compilation
   - Mitigation: Review gobuild Config thoroughly
3. **Server execution**: Working directory changes might affect runtime
   - Mitigation: Integration tests with actual server execution

### Timeline Risk
- **Estimated effort**: 4-6 hours
- **Dependencies**: None (self-contained)
- **Blocker potential**: Low

---

## Success Criteria

✅ All existing tests pass with updated Config  
✅ golite integration works without issues  
✅ Clearer, more intuitive API  
✅ Reduced configuration complexity (3 fewer fields)  
✅ Consistent with tinywasm/goflare patterns  
✅ Comprehensive documentation updates  
✅ Clear migration guide available  
✅ No backward compatibility layer needed  
✅ Template generation works with new SourceDir  

---

## Final Summary

### Approved Changes
- **REMOVE**: `RootFolder` → **ADD**: `SourceDir` (relative path, clearer semantics)
- **REMOVE**: `MainFileWithoutExtension` → **CONVENTION**: Always use "main.go"
- **REMOVE**: `PublicFolder` → Not needed (template saves to SourceDir)
- **ADD**: `OutputDir` → Explicit compilation and execution directory
- **ADD**: `NewConfig()` → Helper function with sensible defaults
- **BREAKING**: No backward compatibility layer

### Benefits Achieved
1. **Reduced complexity**: 8 fields → 8 fields (but 3 removed, 2 added, net simplification)
2. **Clearer semantics**: SourceDir/OutputDir pattern matches industry standards
3. **Convention over configuration**: main.go is standard
4. **Better separation**: Source vs build artifacts clearly separated
5. **Consistency**: Matches tinywasm and goflare patterns
6. **Easier to understand**: Less cognitive load for users

### Migration Impact
- **Affected packages**: golite (primary user)
- **Breaking changes**: YES - All Config instantiations must update
- **Estimated migration time**: 15-30 minutes per package
- **Risk level**: LOW - Clear migration path, internal usage only

---

## Next Steps - Ready to Implement ✅

All decisions have been approved. Ready to proceed with implementation:

1. ✅ **START**: Phase 2 - Core Changes (goserver package)
2. ⏳ **THEN**: Phase 3 - Update Dependents (tests + golite)
3. ⏳ **THEN**: Phase 4 - Validation (testing)
4. ⏳ **THEN**: Phase 5 - Documentation (README, CHANGELOG)

---

**Status**: ✅ **APPROVED - Ready to implement**

**Estimated completion**: 4-6 hours

**Dependencies**: None

**Blockers**: None - All questions answered
