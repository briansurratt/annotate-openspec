## ADDED Requirements

### Requirement: Parse note into three-part document model
The system SHALL parse a markdown file into a three-part model: `PreFence` (bytes before the opening `---` fence, may be empty), `Frontmatter` (a `map[string]any` decoded from the YAML block between the opening and closing `---` fences), and `Body` (the raw string after the closing `---` fence). If no frontmatter fence is present the file SHALL be treated as having an empty `PreFence`, an empty `Frontmatter` map, and a `Body` equal to the full file contents.

#### Scenario: File with frontmatter
- **WHEN** a file begins with `---\n`, contains valid YAML, then a closing `---\n`
- **THEN** `PreFence` is empty, `Frontmatter` contains the decoded key-value pairs, and `Body` is the remaining markdown content

#### Scenario: File with pre-fence content
- **WHEN** a file has text before the first `---\n` line
- **THEN** `PreFence` contains that text verbatim, `Frontmatter` is decoded from the YAML block, and `Body` is the content after the closing fence

#### Scenario: File with no frontmatter fence
- **WHEN** a file contains no `---` delimiters
- **THEN** `PreFence` is empty, `Frontmatter` is an empty map, and `Body` is the full file contents

#### Scenario: File with empty frontmatter block
- **WHEN** a file has `---\n---\n` with nothing between the fences
- **THEN** `Frontmatter` is an empty map and `Body` is the content after the closing fence

### Requirement: Round-trip fidelity
The system SHALL reproduce the original byte sequence when a parsed note is serialized back to disk without any frontmatter modifications.

#### Scenario: Unmodified round-trip
- **WHEN** a note is parsed and immediately written back without changing any field
- **THEN** the resulting file bytes are identical to the original file bytes

### Requirement: Atomic write
The system SHALL write note changes atomically by first writing to `<filepath>.tmp` in the same directory, then renaming to the target path using `os.Rename`.

#### Scenario: Write completes successfully
- **WHEN** `Write` is called with valid note content
- **THEN** the target file reflects the new content and no `.tmp` file remains on disk

#### Scenario: Process crash during write
- **WHEN** the process is interrupted after writing `.tmp` but before `os.Rename`
- **THEN** the original target file is unchanged and only a stale `.tmp` file remains

### Requirement: Content hash excludes ai_suggestions
The system SHALL compute a content hash as SHA-256 over the concatenation of: the YAML serialization of frontmatter keys sorted alphabetically with `ai_suggestions` excluded, and the raw body string. The hash SHALL be identical for two notes that differ only in their `ai_suggestions` value.

#### Scenario: Hash stability across ai_suggestions changes
- **WHEN** two notes are identical except one has a non-empty `ai_suggestions` frontmatter value
- **THEN** both notes produce the same content hash

#### Scenario: Hash changes on body edit
- **WHEN** a note's body text is modified
- **THEN** the content hash differs from the hash before the edit

#### Scenario: Hash changes on non-ai_suggestions frontmatter edit
- **WHEN** a frontmatter key other than `ai_suggestions` is modified
- **THEN** the content hash differs from the hash before the edit

### Requirement: Frontmatter re-serialization preserves insertion order with ai_suggestions last
When writing a note back to disk, the system SHALL emit frontmatter keys in their original insertion order, except that `ai_suggestions` SHALL always appear as the final key regardless of its original position.

#### Scenario: ai_suggestions written last
- **WHEN** a note with `ai_suggestions` interspersed among other keys is written to disk
- **THEN** `ai_suggestions` appears after all other frontmatter keys in the output file
