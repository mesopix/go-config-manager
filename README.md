# go-config-manager

[![Go Reference](https://pkg.go.dev/badge/github.com/mesopix/go-config-manager.svg)](https://pkg.go.dev/github.com/mesopix/go-config-manager)

Per-app configuration stored as JSON in the user config directory. Zero dependencies.

---

# For Users

## Install

```sh
go get github.com/mesopix/go-config-manager
```

```go
import "github.com/mesopix/go-config-manager"
```

## Usage

Create a `default_config.json` template file and embed it:

```go
//go:embed default_config.json
var defaultConfigJSON []byte

c, err := configmanager.LoadAppConfig("myapp", defaultConfigJSON)
if err != nil {
    // 配置文件损坏时返回 *CorruptConfigError，不提供默认值降级；
    // 调用方应打印错误并退出，或引导用户修复。
    var corruptErr *configmanager.CorruptConfigError
    if errors.As(err, &corruptErr) {
        fmt.Fprintf(os.Stderr, "config file %s is corrupt: %v\n", corruptErr.Path, corruptErr.Err)
        os.Exit(1)
    }
    log.Fatal(err)
}

port, _ := c.Get("port")
c.Set("port", 9090)
if err := c.Save(); err != nil { ... }
```

The file lives at `<user config dir>/myapp/config.json`; on first run it is created from the embedded JSON template. `c.Path()` returns the absolute path (useful for editor integrations or error messages).

Note: JSON numbers come back as `float64` from `Get`.

### Struct binding

For typed access, use `DecodeFields` / `SetFieldsFrom` with a struct whose fields carry `json` tags:

```go
type Settings struct {
    Host string  `json:"host"`
    Port float64 `json:"port"`
}

var s Settings
if err := c.DecodeFields(&s); err != nil { ... }
s.Port = 9090
if err := c.SetFieldsFrom(s); err != nil { ... }
if err := c.Save(); err != nil { ... }
```

Missing keys leave target fields at their zero value; pointer fields stay `nil` when absent and become non-nil when explicitly set (even to the zero value), preserving the "unset vs explicit zero" distinction.

### Schema validation

```go
schema := configmanager.Schema{
    "host": {Type: configmanager.TypeString, Required: true},
    "port": {Type: configmanager.TypeFloat, Default: float64(3000)},
}

switch c.Check(schema) {
case configmanager.Valid:
    // nothing to do
case configmanager.MissingDefaults, configmanager.ExtraFields, configmanager.MissingAndExtra:
    if err := c.Normalize(schema); err != nil { ... }
    if err := c.Save(); err != nil { ... }
case configmanager.Invalid:
    log.Fatal("required field missing or type mismatch")
}
```

`Normalize` fills missing defaults and removes extra fields; it is a no-op when already `Valid`.

### Repair (not yet implemented)

`RepairAppConfig(appName, defaultJSON)` is reserved for future versions. It currently returns an error indicating the feature is not implemented.

## API Reference

Full API documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/mesopix/go-config-manager).

---

# For Developers

## Project Structure

```
.
├── config.go           # Core library: LoadAppConfig, Config, Get/Set/Save,
│                       # DecodeFields/SetFieldsFrom, Check/Normalize, Path,
│                       # CorruptConfigError, RepairAppConfig (stub)
├── schema.go           # Schema types, Check, Normalize (standalone)
├── config_test.go      # Internal tests
├── api_test.go         # External black-box tests
└── examples/
    └── demo/
        ├── default_config.json  # Embedded default config template
        └── main.go              # Runnable demo
```

Single package at module root, zero external dependencies.

## Development Guide

Run tests and static analysis:

```sh
go test -v ./...
go vet ./...
```

Tests use `t.TempDir()` and override `AppData` / `XDG_CONFIG_HOME` / `HOME` via `t.Setenv`, so they never touch real user config.

## Design Decisions

- **No auto-detection of executable name**: Renaming the binary would silently create a new config file, losing previous settings. Test binaries would also use different config paths. Multiple binaries in the same project often share one config. The caller explicitly provides `appName`.
- **Re-read after first save**: On first run, `LoadAppConfig` writes the embedded JSON defaults to disk then reads it back. This ensures numeric types are always `float64` (matching subsequent runs), avoiding subtle type mismatches between first and later launches.
- **Corrupt files fail loudly**: When an existing config file cannot be read or parsed, `LoadAppConfig` returns `nil` and a `*CorruptConfigError` (carrying the file path and original error). It does NOT fall back to defaults — callers must surface the error to the user rather than silently continuing with stale defaults. The bad file is never overwritten; `RepairAppConfig` is reserved for future repair workflows.
- **Atomic save**: `Save()` writes to a temp file in the same directory, calls `Sync()` to flush to disk, then `Rename`s over the target. A crash during save never leaves a half-written config.
- **Zero dependencies**: Only stdlib (`encoding/json`, `os`, `path/filepath`). Keeps the dependency tree minimal for a utility library.

## Release Process

1. Ensure all tests pass: `go test -v ./... && go vet ./...`
2. Commit all changes to `main`.
3. Create and push a semantic version tag:

   ```sh
   git tag v0.x.y
   git push origin main --tags
   ```

4. pkg.go.dev will automatically pick up the new version within minutes.

> ⚠️ Published tags are immutable — never delete or move them. Use a new version number for any change.
