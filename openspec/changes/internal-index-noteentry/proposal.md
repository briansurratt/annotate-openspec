## Why

The annotation daemon needs an in-memory index of all notes in the workspace to assemble prompts with title/alias lookups, backlink counts, and mtime-sorted context sections. Without this index, the worker has no efficient way to find candidate links or build tag catalogs at prompt time.

## What Changes

- Introduce `NoteEntry` struct in `internal/index` to represent a single note's indexed metadata (path, title, aliases, tags, mtime, content hash)
- Build an in-memory map (`map[string]*NoteEntry` keyed by path) populated on startup via mtime scan
- Bootstrap optimization: compare scanned mtimes against an SQLite cache to skip re-parsing unchanged notes
- Add dirty tracking so the `indexFlusher` goroutine can flush only changed entries to SQLite

## Capabilities

### New Capabilities
- `note-index`: In-memory map of `NoteEntry` structs with startup mtime scan, SQLite cache for fast restarts, and dirty-flag tracking for periodic flushing

### Modified Capabilities
- `database-store`: Add `index_cache` table schema and read/write operations for cached index entries

## Impact

- New package: `internal/index`
- Modifies: `internal/store` (adds `index_cache` table migration)
- Consumed by: `internal/daemon` (wires `indexFlusher` goroutine), `internal/worker` (reads index for prompt assembly)
- No external API or CLI changes
