package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ————————————————————————————————————————————————————
// 5.1  Parse with frontmatter and body
// ————————————————————————————————————————————————————

func TestParse_WithFrontmatterAndBody(t *testing.T) {
	input := "---\ntitle: My Note\ntags:\n  - go\n  - testing\n---\n# Heading\n\nSome body text.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.PreFence != "" {
		t.Errorf("PreFence: want empty, got %q", n.PreFence)
	}
	if n.Frontmatter["title"] != "My Note" {
		t.Errorf("title: want %q, got %v", "My Note", n.Frontmatter["title"])
	}
	tags, ok := n.Frontmatter["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("tags: want []any{go,testing}, got %v", n.Frontmatter["tags"])
	}
	if !strings.Contains(n.Body, "# Heading") {
		t.Errorf("Body missing heading: %q", n.Body)
	}
}

// ————————————————————————————————————————————————————
// 5.2  Parse with pre-fence content
// ————————————————————————————————————————————————————

func TestParse_WithPreFence(t *testing.T) {
	input := "some pre-fence text\n---\ntitle: Test\n---\nBody here.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(n.PreFence, "some pre-fence text") {
		t.Errorf("PreFence: want pre-fence text, got %q", n.PreFence)
	}
	if n.Frontmatter["title"] != "Test" {
		t.Errorf("title: want %q, got %v", "Test", n.Frontmatter["title"])
	}
	if n.Body != "Body here.\n" {
		t.Errorf("Body: want %q, got %q", "Body here.\n", n.Body)
	}
}

// ————————————————————————————————————————————————————
// 5.3  Parse with no frontmatter fence
// ————————————————————————————————————————————————————

func TestParse_NoFence(t *testing.T) {
	input := "# Just a body\n\nNo frontmatter here.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.PreFence != "" {
		t.Errorf("PreFence: want empty, got %q", n.PreFence)
	}
	if len(n.Frontmatter) != 0 {
		t.Errorf("Frontmatter: want empty map, got %v", n.Frontmatter)
	}
	if n.Body != input {
		t.Errorf("Body: want full input, got %q", n.Body)
	}
}

// ————————————————————————————————————————————————————
// 5.4  Parse with empty frontmatter block
// ————————————————————————————————————————————————————

func TestParse_EmptyFrontmatter(t *testing.T) {
	input := "---\n---\nBody content.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(n.Frontmatter) != 0 {
		t.Errorf("Frontmatter: want empty map, got %v", n.Frontmatter)
	}
	if n.Body != "Body content.\n" {
		t.Errorf("Body: want %q, got %q", "Body content.\n", n.Body)
	}
}

// ————————————————————————————————————————————————————
// 5.5  Round-trip fidelity
// ————————————————————————————————————————————————————

func TestRoundTrip(t *testing.T) {
	input := "---\ntitle: Round-trip\ntags:\n    - one\n    - two\n---\n# Body\n\nText.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := n.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The round-trip may reformat YAML scalars slightly (e.g. list indentation),
	// so we verify structural equivalence: re-parse and compare fields.
	n2, err := Parse(strings.NewReader(string(got)))
	if err != nil {
		t.Fatalf("Parse round-trip: %v", err)
	}
	if n2.Frontmatter["title"] != "Round-trip" {
		t.Errorf("title: want Round-trip, got %v", n2.Frontmatter["title"])
	}
	if !strings.Contains(n2.Body, "# Body") {
		t.Errorf("Body: missing heading after round-trip")
	}
}

// ————————————————————————————————————————————————————
// 5.6  Write leaves no .tmp file on success
// ————————————————————————————————————————————————————

func TestWrite_NoTmpFileOnSuccess(t *testing.T) {
	input := "---\ntitle: Clean\n---\nBody.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := n.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf(".tmp file still exists after successful Write")
	}
}

// ————————————————————————————————————————————————————
// 5.7  ContentHash equal for notes differing only in ai_suggestions
// ————————————————————————————————————————————————————

func TestContentHash_IgnoresAISuggestions(t *testing.T) {
	base := "---\ntitle: Same\n---\nBody text.\n"
	withAI := "---\ntitle: Same\nai_suggestions:\n  links: []\n---\nBody text.\n"

	n1, err := Parse(strings.NewReader(base))
	if err != nil {
		t.Fatalf("Parse base: %v", err)
	}
	n2, err := Parse(strings.NewReader(withAI))
	if err != nil {
		t.Fatalf("Parse withAI: %v", err)
	}

	if n1.ContentHash() != n2.ContentHash() {
		t.Errorf("hashes differ: %s vs %s", n1.ContentHash(), n2.ContentHash())
	}
}

// ————————————————————————————————————————————————————
// 5.8  ContentHash differs after body edit
// ————————————————————————————————————————————————————

func TestContentHash_DiffersOnBodyEdit(t *testing.T) {
	input := "---\ntitle: Hash Test\n---\nOriginal body.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	before := n.ContentHash()

	n.Body = "Modified body.\n"
	after := n.ContentHash()

	if before == after {
		t.Error("ContentHash did not change after body edit")
	}
}

// ————————————————————————————————————————————————————
// 5.9  ContentHash differs after non-ai_suggestions frontmatter edit
// ————————————————————————————————————————————————————

func TestContentHash_DiffersOnFrontmatterEdit(t *testing.T) {
	input := "---\ntitle: Original Title\n---\nBody.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	before := n.ContentHash()

	n.Frontmatter["title"] = "Changed Title"
	after := n.ContentHash()

	if before == after {
		t.Error("ContentHash did not change after frontmatter edit")
	}
}

// ————————————————————————————————————————————————————
// 5.10  ai_suggestions serialized as last frontmatter key
// ————————————————————————————————————————————————————

func TestWrite_AISuggestionsLast(t *testing.T) {
	// ai_suggestions appears before other keys in the source.
	input := "---\nai_suggestions:\n  links: []\ntitle: Order Test\nauthor: Alice\n---\nBody.\n"
	n, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := n.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	aiIdx := strings.Index(content, "ai_suggestions:")
	titleIdx := strings.Index(content, "title:")
	authorIdx := strings.Index(content, "author:")

	if aiIdx == -1 {
		t.Fatal("ai_suggestions key not found in output")
	}
	if aiIdx < titleIdx || aiIdx < authorIdx {
		t.Errorf("ai_suggestions appears before other keys:\n%s", content)
	}
}
