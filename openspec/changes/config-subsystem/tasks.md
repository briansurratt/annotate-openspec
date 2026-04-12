## 1. Package Scaffolding

- [x] 1.1 Create `internal/config/` directory and `config.go` file with package declaration
- [x] 1.2 Define `Config` struct with all §13 PRD fields and `yaml` struct tags
- [x] 1.3 Define `NamespacePolicy` struct (max_links, max_tags, excluded, adjacent_notes_count, date_format)
- [x] 1.4 Define `AIProviderConfig` struct (provider name, model, api_key_env, max_tokens)
- [x] 1.5 Define default constants for link caps (daily.journal: 8, projects: 12, reference: 8, technology: 10, default: 8) and debounce (500ms)

## 2. Loading and Decoding

- [x] 2.1 Implement `Load(path string) (*Config, error)` using `yaml.NewDecoder` with `KnownFields(true)`
- [x] 2.2 Return a descriptive error when the file does not exist (wrap `os.Open` error)
- [x] 2.3 Return the decoder error verbatim when an unknown key is encountered

## 3. Defaults and Validation

- [x] 3.1 Implement `applyDefaults(c *Config)` that sets debounce interval and per-namespace link/tag cap defaults when zero-valued
- [x] 3.2 Implement `validate(c *Config) error` that checks `workspace_path` is non-empty and all duration strings parse correctly
- [x] 3.3 Wire `applyDefaults` and `validate` into `Load` after successful decode

## 4. Per-Namespace Policy

- [x] 4.1 Implement `(c *Config) PolicyFor(namespace string) NamespacePolicy` using `path.Match` to iterate namespace_policies overrides
- [x] 4.2 Ensure `PolicyFor` returns global defaults when no override glob matches
- [x] 4.3 Ensure `PolicyFor` sets `Excluded: true` when the namespace matches any entry in `excluded_namespaces`

## 5. Tests

- [x] 5.1 Write test for successful load of a fully-populated fixture config file
- [x] 5.2 Write test for unknown-key rejection (strict decoding)
- [x] 5.3 Write test for default application (absent `debounce_interval` → 500ms; explicit `0` → 500ms)
- [x] 5.4 Write test for `validate` with missing `workspace_path` and invalid duration string
- [x] 5.5 Write table-driven tests for `PolicyFor`: matching override, no match, excluded namespace
- [x] 5.6 Write round-trip test: marshal a `Config` to YAML, load it back, assert equality
