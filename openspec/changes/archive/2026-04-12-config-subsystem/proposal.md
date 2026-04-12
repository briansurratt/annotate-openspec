## Why

The daemon and all subsystems need a validated, strongly-typed view of `.annotate/config.yaml` before they can do any work. Without a config package, every subsystem would independently parse YAML and silently ignore misconfigured keys — the `internal/config` package establishes a single load-and-validate gate that fails fast on unknown keys and applies defaults.

## What Changes

- Introduces `internal/config` package with a typed `Config` struct covering all §13 PRD fields (namespaces, link caps, exclusions, debounce interval, retention, AI provider settings, etc.)
- Loads `.annotate/config.yaml` using `gopkg.in/yaml.v3` with strict decoding (fail on unknown keys)
- Validates required fields, applies defaults for optional fields (e.g. `debounce_interval: 500ms`)
- Exposes a `Load(path string) (*Config, error)` function consumed by daemon startup
- Provides per-namespace policy access helpers used by the worker and prompt assembly

## Capabilities

### New Capabilities

- `config-loader`: YAML config struct definition, `Load()` function with strict decoding and validation, defaults application, and per-namespace policy lookup helpers

### Modified Capabilities

<!-- No existing spec-level requirements are changing -->

## Impact

- New package: `internal/config`
- Consumed by: `internal/daemon` (startup), `internal/worker` (namespace policy), `internal/eventlog` (retention window), `internal/queue` (debounce interval)
- Dependencies: `gopkg.in/yaml.v3` (already required per implementation plan)
- No breaking changes; this is a new foundational package
