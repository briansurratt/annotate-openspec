## Why

The `internal/notes` package needs to parse and manipulate Obsidian-style markdown notes with YAML frontmatter. Without a reliable document model, later milestones cannot safely read or write `ai_suggestions` and other metadata without corrupting note content.

## What Changes

- Introduce a three-part document model: pre-fence content (anything before the opening `---`), a parsed frontmatter map, and the raw body after the closing `---`
- Implement frontmatter parsing using `gopkg.in/yaml.v3`
- Implement atomic writes via a `<file>.tmp` → `os.Rename` pattern to prevent partial writes
- Implement content hashing over the body and all non-`ai_suggestions` frontmatter keys, enabling change detection that ignores daemon-generated annotations

## Capabilities

### New Capabilities

- `note-frontmatter-parsing`: Parse and represent markdown notes as a three-part model (pre-fence / frontmatter map / raw body), with atomic write and content hash support

### Modified Capabilities

<!-- none -->

## Impact

- New package: `internal/notes`
- Dependency: `gopkg.in/yaml.v3` (already listed in prerequisites)
- Consumed by: `internal/index`, `internal/worker`, `internal/merge`, `internal/daemon` (apply action) in later milestones
