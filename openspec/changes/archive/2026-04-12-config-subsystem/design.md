## Context

The `annotate` daemon requires a single, authoritative view of `.annotate/config.yaml` before any subsystem initialises. At daemon startup, config is loaded once and passed as a value (or pointer) into the daemon, worker, eventlog, and queue components. There is no runtime reload — a restart is required to pick up config changes.

The §13 PRD fields cover: workspace path, namespace definitions (glob patterns, link caps, tag caps, exclude flags), AI provider selection and credentials, debounce interval, event log retention, and adjacent-notes settings.

## Goals / Non-Goals

**Goals:**
- Define a complete, strongly-typed `Config` struct that maps 1-to-1 with `.annotate/config.yaml`
- Fail loudly on unrecognised YAML keys (no silent misconfiguration)
- Apply sensible defaults for optional fields (e.g. `debounce_interval: 500ms`, link cap values from §11)
- Provide a `Load(path string) (*Config, error)` entry point for daemon startup
- Expose a `PolicyFor(namespace string) NamespacePolicy` helper that resolves per-namespace overrides via glob matching

**Non-Goals:**
- Runtime config hot-reload (restart required)
- Config file generation / templating
- Validation of AI provider credentials (done at daemon health check, not load time)
- Persistent config storage (YAML file only)

## Decisions

### 1. Strict YAML decoding via `yaml.v3` `KnownFields`

**Decision**: Use `yaml.Decoder` with `KnownFields(true)` rather than plain `yaml.Unmarshal`.

**Rationale**: Silent misconfiguration is a primary operational pain point. Unknown keys indicate typos or stale config from a previous version. Strict decoding turns these into errors at startup instead of mysteries at runtime.

**Alternative considered**: Warn on unknown keys (log + continue). Rejected because warnings are easily missed; a hard error forces the operator to fix config before the daemon runs.

### 2. Two-pass defaults: struct zero-values then explicit `applyDefaults()`

**Decision**: Define default values as constants and apply them in an explicit `applyDefaults(*Config)` function called after decode, before validation.

**Rationale**: `yaml.v3` does not call constructors; zero-values for numeric fields (0) are valid YAML but wrong defaults (e.g. 0ms debounce). An explicit `applyDefaults` pass makes defaults auditable and testable independently of parsing.

**Alternative considered**: Embed defaults as YAML and merge with user config. Rejected — more complex, harder to audit which fields have defaults.

### 3. Glob-based namespace policy lookup via `github.com/gobwas/glob` (or stdlib `path.Match`)

**Decision**: Resolve per-namespace policy overrides at call time using `path.Match` against the namespace string (e.g. `"daily.journal.2024-01-15"` matches `"daily.journal.*"`).

**Rationale**: Per-namespace overrides are an open-ended glob map. Evaluating at call time with `path.Match` requires no additional indexing and is fast enough for per-file use.

**Alternative considered**: Pre-compile globs at load time. Acceptable future optimisation; premature for M1 where the map is small.

### 4. Flat validation pass, no schema generation at load time

**Decision**: Validate in a hand-written `validate(*Config) error` function; do not use `invopop/jsonschema` here.

**Rationale**: `invopop/jsonschema` is used for AI provider output schemas (M3). Applying it to config adds an indirect dependency and schema round-trip without benefit — the YAML struct tags are sufficient documentation.

## Risks / Trade-offs

- **Strict decoding rejects comments-only YAML** — not a real risk; `yaml.v3` handles this correctly, but worth a smoke test.
- **`path.Match` is not a full glob** (`**` is not supported) → Mitigation: document that namespace globs use single-`*` wildcard only, matching the §13 PRD wording. If `**` becomes necessary, swap to `gobwas/glob`.
- **No reload means config drift** — operators must restart the daemon after editing config. This is acceptable for M1; M4 adds `--dry-run` mode which implicitly re-reads config per invocation.
- **Zero-value ambiguity for numeric fields** — a user who explicitly sets `debounce_interval: 0` will get the default (500ms) overwritten. Mitigation: document that `0` means "use default"; add a comment in the example config.
