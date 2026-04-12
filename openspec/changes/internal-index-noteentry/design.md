## Context

The annotation worker needs a fast in-memory lookup of all workspace notes to build prompt context: title/alias matching for link suggestions, backlink counts, tag catalogs with sample titles, and mtime-sorted index truncation. Without a pre-built index, each worker invocation would have to scan and parse the entire workspace on demand.

The `internal/store` package already defines the `index_cache` table schema (via M1 Store item 1). This design defines how `internal/index` reads from and writes to that cache, and how it maintains the in-memory map.

## Goals / Non-Goals

**Goals:**
- Define `NoteEntry` struct covering all fields needed for prompt assembly
- Populate the in-memory map at startup using mtime comparison against the SQLite cache to avoid re-parsing unchanged notes
- Track dirty entries so the `indexFlusher` goroutine can flush incrementally
- Expose a minimal, thread-safe API for the worker and daemon to read/update the index

**Non-Goals:**
- fsnotify watch integration (wired in `internal/daemon`, not in `internal/index`)
- Prompt assembly logic (belongs in `internal/worker`)
- Any AI or annotation logic

## Decisions

### NoteEntry fields
`NoteEntry` holds: `Path string`, `Title string`, `Aliases []string`, `Tags []string`, `Mtime time.Time`, `Hash string`, `Headings []string`, `BacklinkCount int`. `BacklinkCount` is computed by the index after all entries are loaded (count how many entries link to this path). `Headings` enables the section-heading index context section.

**Alternatives considered**: embedding the raw frontmatter map — rejected because it leaks parsing details into consumers; embedding body text — rejected, too large for an in-memory map.

### Cache strategy: mtime-based staleness
On startup, load all rows from `index_cache`, then stat every `.md` file. If `stat.ModTime() <= cached.Mtime`, use the cached `NoteEntry` (deserialize from the `data` JSON column). If newer or absent, parse the file and mark dirty. This avoids a full re-parse on daemon restart.

**Alternatives considered**: content-hash-based staleness — more accurate but requires reading every file to compute the hash before knowing if re-parse is needed, defeating the purpose.

### Dirty tracking
Each `NoteEntry` wraps a `dirty bool` flag (unexported). `Update(entry)` sets dirty. `indexFlusher` calls `FlushDirty()` which collects dirty entries, writes them to SQLite via `internal/store`, and clears the flag. Flush is also called on graceful shutdown.

### Thread safety
The `Index` struct uses a single `sync.RWMutex`: `RLock` for `Get`/`List`/`All`, `Lock` for `Update`/`Delete`/`FlushDirty`. This is appropriate because writes are infrequent (one per saved file) and reads are frequent (every worker invocation).

### Serialization format for `data` column
JSON via `encoding/json`. The `data` column stores the full `NoteEntry` minus `dirty`. Simple, human-inspectable, no extra deps.

## Risks / Trade-offs

- **Memory footprint**: For large vaults (10k+ notes), the in-memory map could use significant RAM. Mitigation: `NoteEntry` stores only metadata, not body text; fields are small.
- **Startup scan latency**: Stat-ing thousands of files takes time. Mitigation: cache hit rate should be high after first run; a future optimization could parallelize stat calls.
- **BacklinkCount accuracy**: Computed from in-memory aliases/titles, so accuracy depends on all entries being loaded before backlinks are counted. `Build()` does a two-pass load: parse all entries first, compute backlinks second.
