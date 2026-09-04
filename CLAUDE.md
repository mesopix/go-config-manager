# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`go-config-manager` is a zero-dependency Go library holding one process-wide per-app JSON configuration. `Init`/`RegisterDefaults` optionally assemble the storage path and first-run template; `Load` returns the singleton and fills the exported `ConfigManager` variable. Default location: `<os.UserConfigDir>/<executable name>/config.json`. The library handles first-run creation from a registered JSON template, atomic saves, and type-safe access via `Get`/`Set`.

Module path: `github.com/mesopix/go-config-manager` — single package `appconfig` at module root.

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

- `config.go` — Global assembly: `Init()`, `RegisterDefaults()`, `Reset()`, `Load()` filling the exported `ConfigManager` singleton; `Config` struct with `Get()`/`Set()`/`Save()`, `DecodeFields()`/`SetFieldsFrom()` (struct binding), `Check()`/`Normalize()` (schema integration), `Path()`, `CorruptConfigError`, `Repair()` (stub)
- `schema.go` — Schema types (`FieldType`, `FieldDef`, `Schema`, `SchemaFile`), standalone `Check()` and `Normalize()` on raw maps
- `cli.go` — CLI takeover: `HandleCLI()` dispatches the `config` subcommand from `os.Args[1:]`; `config --edit` launches the user's editor (blocking) and validates the file afterwards
- `examples/demo/` — Runnable demo showing embed + RegisterDefaults/Load + HandleCLI usage pattern

**Key design contracts:**

- **First-run re-read**: `Load` writes the registered template to disk then reads back, ensuring numeric types are always `float64` on both first and subsequent runs. This is intentional — do not "optimize" away the re-read.
- **Corrupt files fail loudly**: When an existing config file cannot be read or parsed, `Load` returns `nil` and `*CorruptConfigError`. It does NOT fall back to defaults — callers must surface the error. The bad file is never overwritten; `Repair` is a reserved stub for future repair workflows.
- **Atomic save**: `Save()` uses temp-file → `Sync()` → `Rename` pattern. Never replace with direct `os.WriteFile`.
- **Global assembly contract**: `Init` and `RegisterDefaults` each succeed only once per assembly (repeats return errors); empty `Init` arguments mean defaults; `firstDir` must be absolute, `secondDir` relative without `..` traversal, `fileName` a plain name. `Load` is idempotent and fills the exported `ConfigManager` variable — do not assign `ConfigManager` outside the library. A missing file without a registered template is an error, never a silent empty config. `Reset()` clears assembly and singleton — tests/re-assembly only.
- **Executable name as default subdirectory**: With no `Init`, the config lives under `<os.UserConfigDir>/<executable name>/`. Renaming the binary changes the default path (old config "disappears"; test binaries get their own namespace). Pass an explicit `secondDir` via `Init` when stability matters. This deliberately overturned the library's original "never auto-detect the executable name" contract — do not "restore" it without the owner's decision.
- **Assembly is guarded, instances are not**: exported assembly functions (`Init`/`RegisterDefaults`/`Reset`/`Load`) serialize on `globalMu`; unexported helpers (`configPath`/`createFromDefaults`) assume the caller holds it. `*Config` methods are not synchronized — callers sharing the singleton across goroutines must lock themselves.
- **File permissions**: Directories created with `0700`, files with `0600`. These constants (`configDirMode`, `configFileMode`) are unexported.
- **CLI takeover is opt-in per invocation**: `HandleCLI` acts only when `args[0]` is exactly `config` (case-sensitive); otherwise returns `(false, nil)` and the client's normal flow is untouched. `handled = true` means the client must skip its normal flow and exit (non-zero on error; usage text is embedded in the error).
- **`config --edit` is the manual-repair path**: a missing file is created via the first-run flow (`ensureConfigFile` reuse); an existing file — corrupt or not — is opened as-is so the user can fix it by hand, and the bad file is never overwritten by the library. After the editor exits the file must still be a JSON object, or the command fails without touching the bad content. Editor precedence: `$VISUAL` → `$EDITOR` → platform default (`notepad` / `open -W` / `xdg-open`); the launch function `editLaunchEditor` is an unexported package variable kept injectable for tests.

## Testing Conventions

- **Two test packages, four test files**: `config_test.go`, `schema_test.go`, and `cli_test.go` (internal, package `appconfig`) test unexported functions like `load()` and `editConfig()`. `api_test.go` (external, package `appconfig_test`) tests only the public API. This separation is intentional.
- **Fake editor injection**: `cli_test.go` swaps the unexported package variable `editLaunchEditor` via the `useFakeEditor(t, fake)` helper (restored with `t.Cleanup`) so `--edit` is tested without launching a real editor. External tests never trigger the `--edit` success path — they cannot inject and would launch a real editor.
- **Test isolation via Reset + Init**: Every store-touching test starts with `Reset()` registered in `t.Cleanup`, then `Init` pointed at `t.TempDir()` — internal helpers are `newTestConfig` (config_test.go) and `newTestStore` (cli_test.go), external helpers are `useStore`/`loadStore` (api_test.go). Calling `useStore` again with the same dir simulates a process restart. Only the lazy-assembly test overrides `AppData`, `XDG_CONFIG_HOME`, and `HOME` via `t.Setenv`. Tests never touch real user config directories.
- **Table-driven tests**: Used for negative/error cases (see `TestLoad_invalidJSON`).
- **Comments in Chinese**: All source code and test comments MUST be written in Chinese. English comments are forbidden. Maintain this convention in all new and modified files.

## Release

Tag-based releases on `main`: `git tag v0.x.y && git push origin main --tags`. Published tags are immutable.
