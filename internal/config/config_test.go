package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briansurratt/annotate/internal/config"
	"gopkg.in/yaml.v3"
)

// ── 5.1 Successful load of a fully-populated fixture ─────────────────────────

func TestLoad_FullFixture(t *testing.T) {
	c, err := config.Load("testdata/full.yaml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if c.WorkspacePath != "/home/user/vault" {
		t.Errorf("WorkspacePath = %q, want /home/user/vault", c.WorkspacePath)
	}
	if time.Duration(c.DebounceInterval) != 300*time.Millisecond {
		t.Errorf("DebounceInterval = %v, want 300ms", c.DebounceInterval)
	}
	if time.Duration(c.EventLogRetention) != 720*time.Hour {
		t.Errorf("EventLogRetention = %v, want 720h", c.EventLogRetention)
	}
	if c.AIProvider.Provider != "anthropic" {
		t.Errorf("AIProvider.Provider = %q, want anthropic", c.AIProvider.Provider)
	}
	if c.AIProvider.MaxTokens != 4096 {
		t.Errorf("AIProvider.MaxTokens = %d, want 4096", c.AIProvider.MaxTokens)
	}
	if len(c.ExcludedNamespaces) != 2 {
		t.Errorf("ExcludedNamespaces len = %d, want 2", len(c.ExcludedNamespaces))
	}
}

// ── 5.1 File not found ────────────────────────────────────────────────────────

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("Load: expected error for missing file, got nil")
	}
}

// ── 5.2 Unknown-key rejection (strict decoding) ───────────────────────────────

func TestLoad_UnknownKey(t *testing.T) {
	path := writeTempYAML(t, "workspace_path: /vault\nunknown_field: oops\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error for unknown key, got nil")
	}
}

// ── 5.3 Default application ───────────────────────────────────────────────────

func TestLoad_DefaultDebounceWhenAbsent(t *testing.T) {
	path := writeTempYAML(t, "workspace_path: /vault\n")
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if time.Duration(c.DebounceInterval) != config.DefaultDebounceInterval {
		t.Errorf("DebounceInterval = %v, want %v", c.DebounceInterval, config.DefaultDebounceInterval)
	}
}

func TestLoad_DefaultDebounceWhenZero(t *testing.T) {
	path := writeTempYAML(t, "workspace_path: /vault\ndebounce_interval: 0\n")
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if time.Duration(c.DebounceInterval) != config.DefaultDebounceInterval {
		t.Errorf("DebounceInterval = %v, want %v (default)", c.DebounceInterval, config.DefaultDebounceInterval)
	}
}

func TestLoad_DefaultEventLogRetention(t *testing.T) {
	path := writeTempYAML(t, "workspace_path: /vault\n")
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if time.Duration(c.EventLogRetention) != config.DefaultEventLogRetention {
		t.Errorf("EventLogRetention = %v, want %v", c.EventLogRetention, config.DefaultEventLogRetention)
	}
}

// ── 5.4 Validate: missing workspace_path and invalid duration ─────────────────

func TestLoad_MissingWorkspacePath(t *testing.T) {
	path := writeTempYAML(t, "debounce_interval: 100ms\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error for missing workspace_path, got nil")
	}
}

func TestLoad_InvalidDurationString(t *testing.T) {
	path := writeTempYAML(t, "workspace_path: /vault\ndebounce_interval: fast\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error for invalid duration, got nil")
	}
}

// ── 5.5 PolicyFor: table-driven ───────────────────────────────────────────────

func TestPolicyFor(t *testing.T) {
	cfgYAML := `
workspace_path: /vault
excluded_namespaces:
  - "private.*"
namespace_policies:
  "projects.*":
    max_links: 20
    max_tags: 6
`
	path := writeTempYAML(t, cfgYAML)
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		namespace    string
		wantMaxLinks int
		wantMaxTags  int
		wantExcluded bool
	}{
		// Matches projects.* override → max_links: 20, max_tags: 6
		{"projects.myproject", 20, 6, false},
		// Matches daily.journal category default → max_links: 8
		{"daily.journal.2024-01-15", config.DefaultMaxLinksDaily, config.DefaultMaxTags, false},
		// No match → global defaults
		{"zettelkasten.notes", config.DefaultMaxLinks, config.DefaultMaxTags, false},
		// Matches excluded_namespaces glob
		{"private.diary", config.DefaultMaxLinks, config.DefaultMaxTags, true},
		// Exact namespace prefix match (reference)
		{"reference.books", config.DefaultMaxLinksReference, config.DefaultMaxTags, false},
		// Exact namespace prefix match (technology)
		{"technology.go", config.DefaultMaxLinksTech, config.DefaultMaxTags, false},
	}

	for _, tt := range tests {
		p := c.PolicyFor(tt.namespace)
		if p.MaxLinks != tt.wantMaxLinks {
			t.Errorf("PolicyFor(%q).MaxLinks = %d, want %d", tt.namespace, p.MaxLinks, tt.wantMaxLinks)
		}
		if p.MaxTags != tt.wantMaxTags {
			t.Errorf("PolicyFor(%q).MaxTags = %d, want %d", tt.namespace, p.MaxTags, tt.wantMaxTags)
		}
		if p.Excluded != tt.wantExcluded {
			t.Errorf("PolicyFor(%q).Excluded = %v, want %v", tt.namespace, p.Excluded, tt.wantExcluded)
		}
	}
}

// ── 5.6 Round-trip: marshal → load → equal ───────────────────────────────────

func TestRoundTrip(t *testing.T) {
	original := &config.Config{
		WorkspacePath:    "/vault",
		DebounceInterval: config.Duration(300 * time.Millisecond),
		EventLogRetention: config.Duration(48 * time.Hour),
		IndexCachePath:   "/vault/.annotate/index.sqlite",
		ExcludedNamespaces: []string{"private.*"},
		NamespacePolicies: map[string]config.NamespaceOverride{
			"projects.*": {MaxLinks: 15, MaxTags: 5},
		},
		AIProvider: config.AIProviderConfig{
			Provider:  "anthropic",
			Model:     "claude-opus-4-6",
			APIKeyEnv: "ANTHROPIC_API_KEY",
			MaxTokens: 2048,
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	path := writeTempYAMLBytes(t, data)
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.WorkspacePath != original.WorkspacePath {
		t.Errorf("WorkspacePath mismatch: got %q, want %q", loaded.WorkspacePath, original.WorkspacePath)
	}
	if loaded.DebounceInterval != original.DebounceInterval {
		t.Errorf("DebounceInterval mismatch: got %v, want %v", loaded.DebounceInterval, original.DebounceInterval)
	}
	if loaded.EventLogRetention != original.EventLogRetention {
		t.Errorf("EventLogRetention mismatch: got %v, want %v", loaded.EventLogRetention, original.EventLogRetention)
	}
	if loaded.IndexCachePath != original.IndexCachePath {
		t.Errorf("IndexCachePath mismatch: got %q, want %q", loaded.IndexCachePath, original.IndexCachePath)
	}
	if loaded.AIProvider != original.AIProvider {
		t.Errorf("AIProvider mismatch: got %+v, want %+v", loaded.AIProvider, original.AIProvider)
	}
	if len(loaded.ExcludedNamespaces) != len(original.ExcludedNamespaces) {
		t.Errorf("ExcludedNamespaces len mismatch: got %d, want %d", len(loaded.ExcludedNamespaces), len(original.ExcludedNamespaces))
	}
	if len(loaded.NamespacePolicies) != len(original.NamespacePolicies) {
		t.Errorf("NamespacePolicies len mismatch: got %d, want %d", len(loaded.NamespacePolicies), len(original.NamespacePolicies))
	}
	if got := loaded.NamespacePolicies["projects.*"]; got != original.NamespacePolicies["projects.*"] {
		t.Errorf("NamespacePolicies[projects.*] mismatch: got %+v, want %+v", got, original.NamespacePolicies["projects.*"])
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	return writeTempYAMLBytes(t, []byte(content))
}

func writeTempYAMLBytes(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
