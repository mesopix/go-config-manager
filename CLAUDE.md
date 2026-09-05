# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`go-config-manager` is a zero-dependency Go library managing a per-app JSON configuration through a `Manager` instance. `NewManager()` creates a manager; `Init`/`RegisterDefaults` optionally assemble the storage path and first-run template; `Load` returns the loaded `*Config` (idempotent per manager). Default location: `<os.UserConfigDir>/<executable name>/config.json`. The library handles first-run creation from a registered JSON template, atomic saves, and type-safe access via `Get`/`Set`.

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

- `config.go` — `Manager` struct + `NewManager()`: instance assembly via `Init()`/`RegisterDefaults()`/`Load()`/`Repair()` (stub) methods; `Config` struct with `Get()`/`Set()`/`Save()`, `DecodeFields()`/`SetFieldsFrom()` (struct binding), `Check()`/`Normalize()` (schema integration), `Path()`, `CorruptConfigError`
- `schema.go` — Schema types (`FieldType`, `FieldDef`, `Schema`, `SchemaFile`), standalone `Check()` and `Normalize()` on raw maps
- `cli.go` — CLI takeover: `Manager.HandleCLI()` dispatches the `config` subcommand from `os.Args[1:]`; `config --edit` launches the user's editor (blocking) and validates the file afterwards
- `examples/demo/` — Runnable demo showing embed + RegisterDefaults/Load + HandleCLI usage pattern

**Key design contracts:**

- **First-run re-read**: `Load` writes the registered template to disk then reads back, ensuring numeric types are always `float64` on both first and subsequent runs. This is intentional — do not "optimize" away the re-read.
- **Corrupt files fail loudly**: When an existing config file cannot be read or parsed, `Load` returns `nil` and `*CorruptConfigError`. It does NOT fall back to defaults — callers must surface the error. The bad file is never overwritten; `Repair` is a reserved stub for future repair workflows.
- **Atomic save**: `Save()` uses temp-file → `Sync()` → `Rename` pattern. Never replace with direct `os.WriteFile`.
- **Manager assembly contract**: `Init` and `RegisterDefaults` each succeed only once per manager (repeats return errors; failed attempts do NOT consume the slot — a failed lazy `Load` still allows `Init`, a successful one does not). Empty `Init` arguments mean defaults; `firstDir` must be absolute, `secondDir` relative without `..` traversal, `fileName` a plain name. `Load` is idempotent and caches the `*Config` inside the manager. A missing file without a registered template is an error, never a silent empty config. A `Manager` embeds a mutex — always use it by pointer, never copy it; managers pointing at the same path share no file lock (last `Save` wins).
- **Executable name as default subdirectory**: With no `Init`, the config lives under `<os.UserConfigDir>/<executable name>/`. Renaming the binary changes the default path (old config "disappears"; test binaries get their own namespace). Pass an explicit `secondDir` via `Init` when stability matters. This deliberately overturned the library's original "never auto-detect the executable name" contract — do not "restore" it without the owner's decision.
- **Assembly is guarded, instances are not**: `Manager` assembly methods (`Init`/`RegisterDefaults`/`Load`/`HandleCLI`→`ensureConfigFile`) serialize on the manager's own mutex; unexported helpers (`path`/`createFromDefaults`) assume the caller holds it. `*Config` methods are not synchronized — callers sharing a config across goroutines must lock themselves.
- **File permissions**: Directories created with `0700`, files with `0600`. These constants (`configDirMode`, `configFileMode`) are unexported.
- **CLI takeover is opt-in per invocation**: `HandleCLI` acts only when `args[0]` is exactly `config` (case-sensitive); otherwise returns `(false, nil)` and the client's normal flow is untouched. `shouldClose = true` means the client must skip its normal flow and exit (non-zero on error; usage text is embedded in the error).
- **`config --edit` is the manual-repair path**: a missing file is created via the first-run flow (`ensureConfigFile` reuse); an existing file — corrupt or not — is opened as-is so the user can fix it by hand, and the bad file is never overwritten by the library. After the editor exits the file must still be a JSON object, or the command fails without touching the bad content. Editor precedence: `$VISUAL` → `$EDITOR` → platform default (`notepad` / `open -W` / `xdg-open`); the launcher is the unexported `Manager.editor` field (nil falls back to `launchEditor`, so a zero-value `Manager` works), injectable in internal tests. The canonical demo order is `RegisterDefaults` → `HandleCLI` → `Load`: registering defaults before `HandleCLI` lets first-run `--edit` create the file, and calling `HandleCLI` before `Load` means this run's `Load` sees the repaired file.

## Testing Conventions

- **Two test packages, four test files**: `config_test.go`, `schema_test.go`, and `cli_test.go` (internal, package `appconfig`) test unexported functions like `load()` and `(m *Manager).editConfig()`. `api_test.go` (external, package `appconfig_test`) tests only the public API. This separation is intentional.
- **Fake editor injection**: `cli_test.go` sets the unexported `Manager.editor` field directly (`m.editor = fake`; per-test manager, no save/restore needed) so `--edit` is tested without launching a real editor. External tests never trigger the `--edit` success path — they cannot inject and would launch a real editor.
- **Test isolation via fresh managers**: Every store-touching test builds a fresh `NewManager()` with `Init` pointed at `t.TempDir()` — internal helpers are `newTestConfig` (config_test.go) and `newTestManager` (cli_test.go, also returns the expected config path), external helpers are `newManager`/`loadManager` (api_test.go). Constructing a second manager on the same dir simulates a process restart. Only the lazy-assembly tests override `AppData`, `XDG_CONFIG_HOME`, and `HOME` via `t.Setenv`. Tests never touch real user config directories.
- **Table-driven tests**: Used for negative/error cases (see `TestLoad_invalidJSON`).
- **Comments in Chinese**: All source code and test comments MUST be written in Chinese. English comments are forbidden. Maintain this convention in all new and modified files.

## Release

Tag-based releases on `main`: `git tag v0.x.y && git push origin main --tags`. Published tags are immutable.
