// Package config provides YAML-based configuration loading and validation for
// the annotate daemon. Call Load to obtain a *Config; all subsystems share the
// single instance created at daemon startup.
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Default values applied when a field is absent or zero-valued.
const (
	DefaultDebounceInterval  = 500 * time.Millisecond
	DefaultEventLogRetention = 30 * 24 * time.Hour // 30 days

	// Link cap defaults from §11.
	DefaultMaxLinksDaily     = 8
	DefaultMaxLinksProjects  = 12
	DefaultMaxLinksReference = 8
	DefaultMaxLinksTech      = 10
	DefaultMaxLinks          = 8 // fallback for all other namespaces

	DefaultMaxTags = 10
)

// Config holds all configuration for the annotate daemon.
// It maps 1-to-1 with .annotate/config.yaml.
type Config struct {
	// WorkspacePath is the root directory of the Obsidian vault. Required.
	WorkspacePath string `yaml:"workspace_path"`

	// DebounceInterval is the delay between the last fsnotify event and queue
	// enqueue. Zero value is replaced by DefaultDebounceInterval (500ms).
	// Note: namespace globs use single-* wildcard (path.Match semantics).
	DebounceInterval Duration `yaml:"debounce_interval"`

	// ExcludedNamespaces is a list of glob patterns for namespaces that must
	// never be sent to an AI provider.
	ExcludedNamespaces []string `yaml:"excluded_namespaces"`

	// NamespacePolicies is an open-ended map from namespace glob to per-namespace
	// overrides. Globs use path.Match semantics (single-* only, no **).
	NamespacePolicies map[string]NamespaceOverride `yaml:"namespace_policies"`

	// AIProvider configures the AI provider used for annotation suggestions.
	AIProvider AIProviderConfig `yaml:"ai_provider"`

	// EventLogRetention is how long event log rows are kept before pruning.
	// Zero value is replaced by DefaultEventLogRetention (30 days).
	EventLogRetention Duration `yaml:"event_log_retention"`

	// IndexCachePath overrides the default SQLite cache path for the note index.
	// Defaults to <workspace>/.annotate/index_cache.sqlite when empty.
	IndexCachePath string `yaml:"index_cache_path"`

	// SocketPath is the Unix domain socket path used for IPC between CLI commands
	// and the running daemon. Defaults to ~/.annotate/annotate.sock when empty.
	SocketPath string `yaml:"socket_path"`
}

// NamespaceOverride holds per-namespace policy overrides stored in the config
// file. All fields are optional; zero values mean "use the global default".
type NamespaceOverride struct {
	MaxLinks           int    `yaml:"max_links"`
	MaxTags            int    `yaml:"max_tags"`
	Excluded           bool   `yaml:"excluded"`
	AdjacentNotesCount int    `yaml:"adjacent_notes_count"`
	DateFormat         string `yaml:"date_format"` // Go time format, e.g. "2006-01-02"
}

// NamespacePolicy is the fully-resolved effective policy for a namespace after
// merging global defaults with any matching per-namespace overrides.
type NamespacePolicy struct {
	MaxLinks           int
	MaxTags            int
	Excluded           bool
	AdjacentNotesCount int
	DateFormat         string
}

// AIProviderConfig configures the AI provider used for annotation suggestions.
type AIProviderConfig struct {
	// Provider is one of "anthropic", "openai", or "github".
	Provider string `yaml:"provider"`
	// Model is the model identifier, e.g. "claude-opus-4-6".
	Model string `yaml:"model"`
	// APIKeyEnv is the name of the environment variable holding the API key.
	APIKeyEnv string `yaml:"api_key_env"`
	// MaxTokens caps the tokens sent per request. 0 uses the provider default.
	MaxTokens int `yaml:"max_tokens"`
}

// Duration is a time.Duration that unmarshals from a YAML string (e.g. "500ms").
// An explicit zero value is treated the same as absent and replaced by the
// appropriate default during applyDefaults.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler, parsing duration strings like "500ms".
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML implements yaml.Marshaler, producing the canonical string form.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Load reads and parses the config YAML file at cfgPath. Unknown YAML keys
// cause an error (strict decoding). Defaults are applied for zero-valued
// optional fields, and required fields are validated before returning.
func Load(cfgPath string) (*Config, error) {
	f, err := os.Open(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("open config file %q: %w", cfgPath, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", cfgPath, err)
	}

	applyDefaults(&c)

	if err := validate(&c); err != nil {
		return nil, err
	}

	return &c, nil
}

// applyDefaults fills in zero-valued optional fields with their default values.
func applyDefaults(c *Config) {
	if c.DebounceInterval == 0 {
		c.DebounceInterval = Duration(DefaultDebounceInterval)
	}
	if c.EventLogRetention == 0 {
		c.EventLogRetention = Duration(DefaultEventLogRetention)
	}
	if c.SocketPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.SocketPath = filepath.Join(home, ".annotate", "annotate.sock")
		}
	}
}

// validate checks that all required fields are present and values are in range.
func validate(c *Config) error {
	if c.WorkspacePath == "" {
		return fmt.Errorf("workspace_path is required")
	}
	return nil
}

// PolicyFor returns the effective NamespacePolicy for namespace. Per-namespace
// overrides (stored as glob patterns in NamespacePolicies) are merged on top of
// category defaults from §11. Namespace globs use path.Match semantics
// (single-* wildcard only; ** is not supported).
func (c *Config) PolicyFor(namespace string) NamespacePolicy {
	p := defaultPolicyForNamespace(namespace)

	// Excluded namespaces override everything.
	for _, glob := range c.ExcludedNamespaces {
		if matched, _ := path.Match(glob, namespace); matched {
			p.Excluded = true
			break
		}
	}

	// Apply matching namespace_policies overrides; zero values mean "keep default".
	for glob, override := range c.NamespacePolicies {
		if matched, _ := path.Match(glob, namespace); !matched {
			continue
		}
		if override.Excluded {
			p.Excluded = true
		}
		if override.MaxLinks != 0 {
			p.MaxLinks = override.MaxLinks
		}
		if override.MaxTags != 0 {
			p.MaxTags = override.MaxTags
		}
		if override.AdjacentNotesCount != 0 {
			p.AdjacentNotesCount = override.AdjacentNotesCount
		}
		if override.DateFormat != "" {
			p.DateFormat = override.DateFormat
		}
	}

	return p
}

// defaultPolicyForNamespace returns the §11 default link cap for namespace.
func defaultPolicyForNamespace(namespace string) NamespacePolicy {
	p := NamespacePolicy{
		MaxLinks: DefaultMaxLinks,
		MaxTags:  DefaultMaxTags,
	}
	switch {
	case hasPrefix(namespace, "daily.journal"):
		p.MaxLinks = DefaultMaxLinksDaily
	case hasPrefix(namespace, "projects"):
		p.MaxLinks = DefaultMaxLinksProjects
	case hasPrefix(namespace, "reference"):
		p.MaxLinks = DefaultMaxLinksReference
	case hasPrefix(namespace, "technology"):
		p.MaxLinks = DefaultMaxLinksTech
	}
	return p
}

// hasPrefix reports whether namespace equals pfx or starts with "pfx.".
func hasPrefix(namespace, pfx string) bool {
	return namespace == pfx ||
		len(namespace) > len(pfx) && namespace[:len(pfx)+1] == pfx+"."
}
