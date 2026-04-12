package notes

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Note represents a parsed markdown note in three parts.
type Note struct {
	// PreFence is any content before the opening --- fence (usually empty).
	PreFence string
	// Frontmatter holds the decoded YAML key-value pairs. Insertion order is
	// preserved via the keyOrder field so that round-trips are stable.
	Frontmatter map[string]any
	// Body is the raw markdown content after the closing --- fence.
	Body string

	keyOrder []string // original insertion order of Frontmatter keys
}

// Parse reads a markdown file and splits it into its three parts.
// If the file contains no --- fences, the entire content is placed in Body.
func Parse(r io.Reader) (*Note, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("notes: read: %w", err)
	}

	// Detect and split the frontmatter block.
	preFence, yamlBlock, body, ok := splitFrontmatter(data)
	if !ok {
		// No frontmatter fences found — treat entire file as body.
		return &Note{
			Frontmatter: map[string]any{},
			Body:        string(data),
		}, nil
	}

	fm, order, err := decodeYAML(yamlBlock)
	if err != nil {
		return nil, fmt.Errorf("notes: parse frontmatter: %w", err)
	}

	return &Note{
		PreFence:    preFence,
		Frontmatter: fm,
		Body:        body,
		keyOrder:    order,
	}, nil
}

// ParseFile is a convenience wrapper around Parse for file paths.
func ParseFile(path string) (*Note, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("notes: open %s: %w", path, err)
	}
	defer f.Close()
	return Parse(f)
}

// Write serializes the note and writes it atomically to path using a
// temporary file followed by os.Rename so that partial writes never corrupt
// the original.
func (n *Note) Write(path string) error {
	buf, err := n.marshal()
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return fmt.Errorf("notes: write tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("notes: rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// ContentHash returns a hex-encoded SHA-256 hash of the note's meaningful
// content. The hash excludes the ai_suggestions frontmatter key so that
// daemon-generated annotation writes do not trigger re-processing.
func (n *Note) ContentHash() string {
	h := sha256.New()

	// Collect non-ai_suggestions keys, sort them for determinism.
	keys := make([]string, 0, len(n.Frontmatter))
	for k := range n.Frontmatter {
		if k != "ai_suggestions" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	// Build a map with only those keys and serialize it.
	filtered := make(map[string]any, len(keys))
	for _, k := range keys {
		filtered[k] = n.Frontmatter[k]
	}
	if len(filtered) > 0 {
		enc, _ := yaml.Marshal(filtered)
		h.Write(enc)
	}

	h.Write([]byte(n.Body))
	return hex.EncodeToString(h.Sum(nil))
}

// ————————————————————————————————————————————————————
// internal helpers
// ————————————————————————————————————————————————————

const fence = "---"

// splitFrontmatter finds the opening and closing --- fences in data and
// returns (preFence, yamlBlock, body, true). If no valid fence pair is found
// it returns ("", "", "", false).
func splitFrontmatter(data []byte) (preFence, yamlBlock, body string, ok bool) {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Find the first line that is exactly "---".
	openIdx := -1
	for i, l := range lines {
		if l == fence {
			openIdx = i
			break
		}
	}
	if openIdx == -1 {
		return "", "", "", false
	}

	// Find the closing "---" after the opening fence.
	closeIdx := -1
	for i := openIdx + 1; i < len(lines); i++ {
		if lines[i] == fence {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "", "", "", false
	}

	pre := strings.Join(lines[:openIdx], "\n")
	if openIdx > 0 {
		pre += "\n"
	}

	yamlLines := lines[openIdx+1 : closeIdx]
	yamlStr := strings.Join(yamlLines, "\n")
	if len(yamlLines) > 0 {
		yamlStr += "\n"
	}

	bodyLines := lines[closeIdx+1:]
	bodyStr := strings.Join(bodyLines, "\n")
	// Preserve the trailing newline that Join drops when bodyLines is non-empty.
	if len(bodyLines) > 0 {
		bodyStr += "\n"
	}

	return pre, yamlStr, bodyStr, true
}

// decodeYAML decodes a YAML string into a map, preserving insertion order via
// a yaml.Node walk so we can re-serialize in the same order.
func decodeYAML(yamlStr string) (map[string]any, []string, error) {
	if strings.TrimSpace(yamlStr) == "" {
		return map[string]any{}, nil, nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		return nil, nil, err
	}

	fm := map[string]any{}
	var order []string

	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		mapping := node.Content[0]
		if mapping.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(mapping.Content); i += 2 {
				keyNode := mapping.Content[i]
				valNode := mapping.Content[i+1]
				key := keyNode.Value
				var val any
				if err := valNode.Decode(&val); err != nil {
					return nil, nil, err
				}
				fm[key] = val
				order = append(order, key)
			}
		}
	}

	return fm, order, nil
}

// marshal serializes the note back to bytes. Frontmatter keys are written in
// their original insertion order, with ai_suggestions always last.
func (n *Note) marshal() ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(n.PreFence)
	buf.WriteString(fence + "\n")

	if len(n.Frontmatter) > 0 {
		// Build ordered key list: original order, ai_suggestions last.
		seen := map[string]bool{}
		ordered := make([]string, 0, len(n.keyOrder))
		for _, k := range n.keyOrder {
			if k != "ai_suggestions" {
				ordered = append(ordered, k)
				seen[k] = true
			}
		}
		// Any keys added after parsing (not in keyOrder) go before ai_suggestions.
		for k := range n.Frontmatter {
			if !seen[k] && k != "ai_suggestions" {
				ordered = append(ordered, k)
			}
		}
		if _, hasAI := n.Frontmatter["ai_suggestions"]; hasAI {
			ordered = append(ordered, "ai_suggestions")
		}

		for _, k := range ordered {
			v := n.Frontmatter[k]
			entry := map[string]any{k: v}
			b, err := yaml.Marshal(entry)
			if err != nil {
				return nil, fmt.Errorf("notes: marshal key %q: %w", k, err)
			}
			buf.Write(b)
		}
	}

	buf.WriteString(fence + "\n")
	buf.WriteString(n.Body)

	return buf.Bytes(), nil
}
