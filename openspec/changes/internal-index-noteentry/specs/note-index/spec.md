## ADDED Requirements

### Requirement: NoteEntry struct
The system SHALL define a `NoteEntry` struct in `internal/index` with fields: `Path string`, `Title string`, `Aliases []string`, `Tags []string`, `Mtime time.Time`, `Hash string`, `Headings []string`, `BacklinkCount int`.

#### Scenario: NoteEntry holds note metadata
- **WHEN** a markdown file is parsed into a NoteEntry
- **THEN** the struct contains the file path, frontmatter title, aliases, tags, mtime, content hash, section headings, and backlink count

### Requirement: In-memory index map
The system SHALL maintain an in-memory `map[string]*NoteEntry` keyed by absolute file path, accessible via thread-safe `Get`, `Update`, `Delete`, and `All` methods on an `Index` struct.

#### Scenario: Concurrent read access
- **WHEN** multiple goroutines call `Index.Get` or `Index.All` simultaneously
- **THEN** all reads succeed without data races

#### Scenario: Write serialization
- **WHEN** two goroutines call `Index.Update` concurrently
- **THEN** both updates succeed and the final map state is consistent

### Requirement: Startup mtime scan
The system SHALL populate the in-memory index on startup by scanning all `.md` files in the workspace root, comparing each file's `stat.ModTime()` against the cached mtime in SQLite, and re-parsing only files newer than their cache entry.

#### Scenario: Cache hit skips re-parse
- **WHEN** a file's mtime matches the cached mtime in `index_cache`
- **THEN** the cached `NoteEntry` is loaded from the `data` column without reading the file

#### Scenario: Cache miss triggers re-parse
- **WHEN** a file's mtime is newer than the cached mtime, or no cache entry exists
- **THEN** the file is parsed and the resulting entry is marked dirty for flushing

#### Scenario: Deleted file removed from index
- **WHEN** the workspace scan finds a cache entry for a path that no longer exists on disk
- **THEN** the entry is removed from the in-memory map and deleted from the SQLite cache

### Requirement: BacklinkCount computation
The system SHALL compute `BacklinkCount` for each `NoteEntry` after all entries are loaded, by counting how many other entries contain the entry's path (or any alias) in their link fields.

#### Scenario: Two-pass build
- **WHEN** `Index.Build()` is called
- **THEN** all entries are parsed in a first pass, then backlink counts are computed in a second pass before the index is considered ready

### Requirement: Dirty tracking for indexFlusher
The system SHALL track which `NoteEntry` values have been modified since the last flush. `Index.FlushDirty(store)` SHALL write all dirty entries to the `index_cache` table via `internal/store` and clear their dirty flags.

#### Scenario: Only dirty entries are flushed
- **WHEN** `FlushDirty` is called after updating two entries in a 1000-entry index
- **THEN** exactly two rows are written to SQLite

#### Scenario: Flush on shutdown
- **WHEN** the daemon receives SIGTERM and calls `FlushDirty` before exiting
- **THEN** all in-flight dirty entries are persisted to SQLite
