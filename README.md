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

```go
tpl := map[string]any{"port": 8080, "debug": true}
c, err := configmanager.LoadAppConfig("myapp", tpl) // always returns a config
if err != nil { ... }

port, _ := c.Get("port")
c.Set("port", 9090)
if err := c.Save(); err != nil { ... }
```

The file lives at `<user config dir>/myapp/config.json`; on first run it is created from the template.

Note: JSON numbers come back as `float64` from `Get`.

## API Reference

Full API documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/mesopix/go-config-manager).

---

# For Developers

## Project Structure

```
.
├── config.go           # Core library: LoadAppConfig, Config, Get, Set, Save
├── config_test.go      # Tests
└── examples/
    └── demo/
        └── main.go     # Runnable demo
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
- **Re-read after first save**: On first run, `LoadAppConfig` writes the template to disk then reads it back. This ensures numeric types are always `float64` (matching subsequent runs), avoiding subtle type mismatches between first and later launches.
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
