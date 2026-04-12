## Context

`internal/notes` is the lowest-level package that touches note files on disk. It must round-trip Obsidian-style markdown reliably: parse YAML frontmatter without altering prose, write back changes atomically, and produce a stable content hash so the worker can skip notes whose meaningful content hasn't changed. All later packages (`index`, `worker`, `merge`, `daemon`) depend on this interface.

The implementation plan specifies: `yaml.v3` for parsing, a three-part document model, atomic writes via `.tmp` → `os.Rename`, and a hash over body + non-`ai_suggestions` frontmatter keys.

## Goals / Non-Goals

**Goals:**
- Parse a markdown file into (pre-fence, frontmatter map, raw body) without losing bytes
- Round-trip a note: parse → mutate frontmatter map → write back → same bytes for unchanged sections
- Atomic disk writes that cannot leave a half-written file
- Deterministic content hash that ignores `ai_suggestions` so daemon writes don't trigger re-processing

**Non-Goals:**
- Validating or interpreting frontmatter field semantics (that belongs to callers)
- Handling nested includes, wiki-links, or any other Obsidian-specific syntax
- Caching or watching files (belongs to `internal/index` and `internal/daemon`)

## Decisions

### Three-part document model

**Decision**: Represent a parsed note as `{ PreFence string, Frontmatter map[string]any, Body string }`.

**Rationale**: Separating pre-fence content (rare but present in some Obsidian templates) from the body avoids off-by-one errors when re-serializing. Keeping the frontmatter as a `map[string]any` (decoded via `yaml.v3`) lets callers read and write arbitrary keys without knowing the full schema.

**Alternative considered**: A typed struct with known fields. Rejected because unknown keys would be silently dropped on round-trip, corrupting user frontmatter.

---

### `yaml.v3` for frontmatter decoding

**Decision**: Use `gopkg.in/yaml.v3` `Decoder` with `KnownFields` disabled (default) to decode into `map[string]any`.

**Rationale**: `yaml.v3` preserves insertion order via `yaml.MapSlice` if needed, handles multi-line strings and anchors correctly, and is already a declared dependency. Decoding into `map[string]any` is the safest way to achieve a lossless round-trip.

**Alternative considered**: `yaml.v2`. Rejected — `yaml.v3` has better error reporting and is the current standard.

---

### Atomic write via `.tmp` + `os.Rename`

**Decision**: Write to `<filepath>.tmp` then `os.Rename` to the target.

**Rationale**: `os.Rename` is atomic on POSIX systems within the same filesystem. If the process crashes mid-write, only the `.tmp` file is lost; the original is untouched. This is simpler and more reliable than fdatasync-based approaches for this use case.

**Risk**: On cross-device moves (e.g., `/tmp` on a different mount), `os.Rename` is not atomic. Mitigated by writing `.tmp` in the same directory as the target.

---

### Content hash excludes `ai_suggestions`

**Decision**: Hash = SHA-256 of (sorted non-`ai_suggestions` frontmatter keys serialized as YAML) + raw body.

**Rationale**: The worker must detect user edits, not its own writes. Including `ai_suggestions` in the hash would cause every daemon write to schedule a re-processing cycle. Sorting keys before hashing ensures the hash is deterministic regardless of insertion order.

**Alternative considered**: Hash only the raw body. Rejected — user may update title, tags, or other frontmatter without touching the body.

---

### YAML re-serialization order

**Decision**: Re-serialize frontmatter keys in original insertion order, with `ai_suggestions` always written last.

**Rationale**: Preserving insertion order minimizes diff noise for users who version-control their notes. Placing `ai_suggestions` last makes it visually easy to find and keeps user-authored fields at the top.

## Risks / Trade-offs

- **YAML round-trip fidelity**: `yaml.v3` may reformat certain values (e.g., bare strings that look like booleans). Mitigation: integration tests with a fixture corpus covering edge cases.
- **Pre-fence content**: Files with content before the opening `---` are uncommon but valid. Mitigation: the pre-fence field is preserved verbatim; tests should cover this case.
- **`.tmp` file leaks**: If the process is killed after writing `.tmp` but before `Rename`, stale `.tmp` files accumulate. Mitigation: acceptable for now; cleanup can be added to daemon startup in a later milestone.
