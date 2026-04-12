## 1. Package Scaffold

- [x] 1.1 Create `internal/notes/` directory and `notes.go` file with package declaration
- [x] 1.2 Add `gopkg.in/yaml.v3` to `go.mod` / `go.sum` if not already present

## 2. Document Model

- [x] 2.1 Define `Note` struct with fields `PreFence string`, `Frontmatter map[string]any`, `Body string`
- [x] 2.2 Implement `Parse(r io.Reader) (*Note, error)` that splits file bytes into pre-fence, YAML block, and body
- [x] 2.3 Handle the no-fence case: return empty `PreFence` and `Frontmatter`, full content as `Body`
- [x] 2.4 Handle the empty frontmatter block case (`---\n---\n`)

## 3. Atomic Write

- [x] 3.1 Implement `(n *Note) Write(path string) error` that serializes the note and writes atomically via `<path>.tmp` → `os.Rename`
- [x] 3.2 Serialize frontmatter keys in original insertion order with `ai_suggestions` moved to last position
- [x] 3.3 Reconstruct output as `PreFence + "---\n" + yamlBlock + "---\n" + Body`

## 4. Content Hash

- [x] 4.1 Implement `(n *Note) ContentHash() string` returning hex-encoded SHA-256
- [x] 4.2 Build hash input: alphabetically sorted frontmatter keys excluding `ai_suggestions`, serialized as YAML, concatenated with raw `Body`

## 5. Tests

- [x] 5.1 Test `Parse` with a file that has frontmatter and body (golden fixture)
- [x] 5.2 Test `Parse` with pre-fence content before the opening `---`
- [x] 5.3 Test `Parse` with no frontmatter fence (body-only file)
- [x] 5.4 Test `Parse` with an empty frontmatter block (`---\n---\n`)
- [x] 5.5 Test round-trip fidelity: parse then write produces identical bytes
- [x] 5.6 Test `Write` leaves no `.tmp` file on success
- [x] 5.7 Test `ContentHash` is equal for two notes differing only in `ai_suggestions`
- [x] 5.8 Test `ContentHash` differs after body edit
- [x] 5.9 Test `ContentHash` differs after non-`ai_suggestions` frontmatter edit
- [x] 5.10 Test that `ai_suggestions` is serialized as the last frontmatter key
