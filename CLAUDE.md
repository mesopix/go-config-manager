# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`go-config-manager` is a zero-dependency Go library for per-app JSON configuration storage. Config files live at `<os.UserConfigDir>/<appName>/config.json`. The library handles first-run creation from an embedded JSON template, atomic saves, and type-safe access via `Get`/`Set`.

Module path: `github.com/mesopix/go-config-manager` — single package `configmanager` at module root.

## Commands

```sh
# Run all tests
go test -v ./...

# Run a single test
go test -v -run TestSaveAtomic ./...

# Static analysis
go vet ./...

# Run the demo
go run ./examples/demo/
```

No Makefile, no linter config, no formatter config. Only stdlib dependencies (`encoding/json`, `os`, `path/filepath`).

## Architecture

**Three source files at module root:**

- `config.go` — Core library: `LoadAppConfig()`, `Config` struct, `Get()`, `Set()`, `Save()`, internal `load()`
- `schema.go` — Type definitions only: `FieldType` enum, `FieldDef` struct, `Schema` map type (not yet wired into validation logic)
- `examples/demo/` — Runnable demo showing embed + LoadAppConfig usage pattern

**Key design contracts:**

- **First-run re-read**: `LoadAppConfig` writes defaults to disk then reads back, ensuring numeric types are always `float64` on both first and subsequent runs. This is intentional — do not "optimize" away the re-read.
- **Atomic save**: `Save()` uses temp-file → `Sync()` → `Rename` pattern. Never replace with direct `os.WriteFile`.
- **Explicit appName**: The library never auto-detects executable name. Callers must provide it. This prevents config loss on binary rename.
- **File permissions**: Directories created with `0700`, files with `0600`. These constants (`configDirMode`, `configFileMode`) are unexported.

## Testing Conventions

- **Two test packages**: `config_test.go` (internal, package `configmanager`) tests unexported functions like `load()`. `api_test.go` (external, package `configmanager_test`) tests only the public API. This separation is intentional.
- **Test isolation**: Both test files define their own `useTempConfigDir(t)` helper that overrides `AppData`, `XDG_CONFIG_HOME`, and `HOME` via `t.Setenv`. Tests never touch real user config directories.
- **Table-driven tests**: Used for negative/error cases (see `TestLoad_invalidJSON`).
- **Comments in Chinese**: All source code and test comments MUST be written in Chinese. English comments are forbidden. Maintain this convention in all new and modified files.

## Release

Tag-based releases on `main`: `git tag v0.x.y && git push origin main --tags`. Published tags are immutable.
