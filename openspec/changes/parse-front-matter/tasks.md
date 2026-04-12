## 1. Package Scaffold

- [ ] 1.1 Create `internal/notes/` directory and `notes.go` file with package declaration
- [ ] 1.2 Add `gopkg.in/yaml.v3` to `go.mod` / `go.sum` if not already present

## 2. Document Model

- [ ] 2.1 Define `Note` struct with fields `PreFence string`, `Frontmatter map[string]any`, `Body string`
- [ ] 2.2 Implement `Parse(r io.Reader) (*Note, error)` that splits file bytes into pre-fence, YAML block, and body
- [ ] 2.3 Handle the no-fence case: return empty `PreFence` and `Frontmatter`, full content as `Body`
- [ ] 2.4 Handle the empty frontmatter block case (`---\n---\n`)

## 3. Atomic Write

- [ ] 3.1 Implement `(n *Note) Write(path string) error` that serializes the note and writes atomically via `<path>.tmp` → `os.Rename`
- [ ] 3.2 Serialize frontmatter keys in original insertion order with `ai_suggestions` moved to last position
- [ ] 3.3 Reconstruct output as `PreFence + "---\n" + yamlBlock + "---\n" + Body`

## 4. Content Hash

- [ ] 4.1 Implement `(n *Note) ContentHash() string` returning hex-encoded SHA-256
- [ ] 4.2 Build hash input: alphabetically sorted frontmatter keys excluding `ai_suggestions`, serialized as YAML, concatenated with raw `Body`

## 5. Tests

- [ ] 5.1 Test `Parse` with a file that has frontmatter and body (golden fixture)
- [ ] 5.2 Test `Parse` with pre-fence content before the opening `---`
- [ ] 5.3 Test `Parse` with no frontmatter fence (body-only file)
- [ ] 5.4 Test `Parse` with an empty frontmatter block (`---\n---\n`)
- [ ] 5.5 Test round-trip fidelity: parse then write produces identical bytes
- [ ] 5.6 Test `Write` leaves no `.tmp` file on success
- [ ] 5.7 Test `ContentHash` is equal for two notes differing only in `ai_suggestions`
- [ ] 5.8 Test `ContentHash` differs after body edit
- [ ] 5.9 Test `ContentHash` differs after non-`ai_suggestions` frontmatter edit
- [ ] 5.10 Test that `ai_suggestions` is serialized as the last frontmatter key
