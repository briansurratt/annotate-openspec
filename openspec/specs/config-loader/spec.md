## ADDED Requirements

### Requirement: Config Loading
The system SHALL expose a `Load(path string) (*Config, error)` function that reads and parses a YAML config file from the given path.

#### Scenario: Successful load of valid config
- **WHEN** `Load` is called with a path to a valid `.annotate/config.yaml`
- **THEN** a populated `*Config` struct is returned with no error

#### Scenario: File not found
- **WHEN** `Load` is called with a path that does not exist
- **THEN** an error is returned describing the missing file
- **AND** `nil` is returned for the config pointer

### Requirement: Strict YAML Decoding
The system SHALL reject configuration files that contain unrecognised keys.

#### Scenario: Unknown key in config file
- **WHEN** `.annotate/config.yaml` contains a key not defined in the `Config` struct
- **THEN** `Load` returns an error naming the unknown key
- **AND** no `Config` value is returned

#### Scenario: Valid config with all known keys
- **WHEN** `.annotate/config.yaml` contains only known keys
- **THEN** `Load` succeeds and all values are decoded into the struct

### Requirement: Default Values Application
The system SHALL apply default values for optional configuration fields when they are absent or zero-valued.

#### Scenario: Debounce interval defaults to 500ms
- **WHEN** `debounce_interval` is absent from the config file
- **THEN** the loaded `Config.DebounceInterval` field equals `500ms`

#### Scenario: Link caps default to §11 values
- **WHEN** a namespace definition omits `max_links`
- **THEN** the resolved cap matches the §11 default for that namespace category (`daily.journal.*`: 8, `projects.*`: 12, `reference.*`: 8, `technology.*`: 10, others: 8)

#### Scenario: Explicit zero is treated as absent
- **WHEN** `debounce_interval` is explicitly set to `0`
- **THEN** the loaded value equals the default `500ms`

### Requirement: Config Validation
The system SHALL validate the loaded config and return a descriptive error if required fields are missing or values are out of range.

#### Scenario: Missing workspace path
- **WHEN** `workspace_path` is absent from the config file
- **THEN** `Load` returns an error stating that `workspace_path` is required

#### Scenario: Invalid duration string
- **WHEN** `debounce_interval` is set to a non-parseable duration (e.g. `"fast"`)
- **THEN** `Load` returns an error identifying the field and the invalid value

#### Scenario: All required fields present
- **WHEN** all required fields are present and valid
- **THEN** `Load` returns no validation error

### Requirement: Per-Namespace Policy Resolution
The system SHALL provide a `PolicyFor(namespace string) NamespacePolicy` method on `Config` that returns the effective policy for a given namespace string, merging global defaults with any matching per-namespace override.

#### Scenario: Namespace matches an override glob
- **WHEN** `PolicyFor` is called with a namespace that matches a configured override glob (e.g. `"daily.journal.2024-01-15"` matches `"daily.journal.*"`)
- **THEN** the returned `NamespacePolicy` reflects the override values for matched fields
- **AND** unoverridden fields retain their global defaults

#### Scenario: Namespace matches no override glob
- **WHEN** `PolicyFor` is called with a namespace that matches no override glob
- **THEN** the returned `NamespacePolicy` contains the global default values

#### Scenario: Namespace excluded from processing
- **WHEN** a namespace glob is listed in `excluded_namespaces`
- **AND** `PolicyFor` is called with a matching namespace
- **THEN** `NamespacePolicy.Excluded` is `true`

### Requirement: Config Struct Coverage
The `Config` struct SHALL include fields for all §13 PRD configuration fields, including: `workspace_path`, `debounce_interval`, `excluded_namespaces`, `namespace_policies` (glob map), `ai_provider`, `event_log_retention`, and `index_cache_path`.

#### Scenario: Round-trip serialisation
- **WHEN** a fully-populated `Config` struct is marshalled to YAML and then loaded via `Load`
- **THEN** the resulting struct equals the original
