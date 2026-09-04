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

No Makefile, no linter config, no formatter config. Only stdlib dependencies (`encoding/json`, `os`, `os/exec`, `path/filepath`, `runtime`).

## Architecture

**Source files at module root:**

- `config.go` — Core library: `LoadAppConfig()`, `Config` struct, `Get()`/`Set()`/`Save()`, `DecodeFields()`/`SetFieldsFrom()` (struct binding), `Check()`/`Normalize()` (schema integration), `Path()`, `CorruptConfigError`, `RepairAppConfig()` (stub)
- `schema.go` — Schema types (`FieldType`, `FieldDef`, `Schema`, `SchemaFile`), standalone `Check()` and `Normalize()` on raw maps
- `cli.go` — CLI takeover: `HandleCLI()` dispatches the `config` subcommand from `os.Args[1:]`; `config --edit` launches the user's editor (blocking) and validates the file afterwards
- `examples/demo/` — Runnable demo showing embed + LoadAppConfig + HandleCLI usage pattern

**Key design contracts:**

- **First-run re-read**: `LoadAppConfig` writes defaults to disk then reads back, ensuring numeric types are always `float64` on both first and subsequent runs. This is intentional — do not "optimize" away the re-read.
- **Corrupt files fail loudly**: When an existing config file cannot be read or parsed, `LoadAppConfig` returns `nil` and `*CorruptConfigError`. It does NOT fall back to defaults — callers must surface the error. The bad file is never overwritten; `RepairAppConfig` is a reserved stub for future repair workflows.
- **Atomic save**: `Save()` uses temp-file → `Sync()` → `Rename` pattern. Never replace with direct `os.WriteFile`.
- **Explicit appName**: The library never auto-detects executable name. Callers must provide it. This prevents config loss on binary rename.
- **File permissions**: Directories created with `0700`, files with `0600`. These constants (`configDirMode`, `configFileMode`) are unexported.
- **CLI takeover is opt-in per invocation**: `HandleCLI` acts only when `args[0]` is exactly `config` (case-sensitive); otherwise returns `(false, nil)` and the client's normal flow is untouched. `handled = true` means the client must skip its normal flow and exit (non-zero on error; usage text is embedded in the error).
- **`config --edit` is the manual-repair path**: a missing file is created via the first-run flow (`LoadAppConfig` reuse); a `*CorruptConfigError` is deliberately swallowed so the raw bad file is opened for the user to fix by hand. After the editor exits the file must still be a JSON object, or the command fails without touching the bad content. Editor precedence: `$VISUAL` → `$EDITOR` → platform default (`notepad` / `open -W` / `xdg-open`); the launch function `editLaunchEditor` is an unexported package variable kept injectable for tests.

## Testing Conventions

- **Two test packages, three test files**: `config_test.go` and `cli_test.go` (internal, package `configmanager`) test unexported functions like `load()` and `editConfig()`. `api_test.go` (external, package `configmanager_test`) tests only the public API. This separation is intentional.
- **Fake editor injection**: `cli_test.go` swaps the unexported package variable `editLaunchEditor` via the `useFakeEditor(t, fake)` helper (restored with `t.Cleanup`) so `--edit` is tested without launching a real editor. External tests never trigger the `--edit` success path — they cannot inject and would launch a real editor.
- **Test isolation**: The internal test files share one `useTempConfigDir(t)` helper (defined in `config_test.go`); `api_test.go` defines its own copy since external packages cannot access it. The helper overrides `AppData`, `XDG_CONFIG_HOME`, and `HOME` via `t.Setenv`. Tests never touch real user config directories.
- **Table-driven tests**: Used for negative/error cases (see `TestLoad_invalidJSON`).
- **Comments in Chinese**: All source code and test comments MUST be written in Chinese. English comments are forbidden. Maintain this convention in all new and modified files.

## Release

Tag-based releases on `main`: `git tag v0.x.y && git push origin main --tags`. Published tags are immutable.
