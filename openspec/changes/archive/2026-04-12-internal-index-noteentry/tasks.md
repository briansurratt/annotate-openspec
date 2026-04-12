## 1. Store: Index Cache Operations

- [x] 1.1 Write failing tests for `LoadIndexCache`, `UpsertIndexCache`, and `DeleteIndexCache` in `internal/store`
- [x] 1.2 Implement `IndexCacheRow` struct with `FilePath`, `Mtime`, `Hash`, `Data` fields
- [x] 1.3 Implement `LoadIndexCache(db *sql.DB) ([]IndexCacheRow, error)` using a prepared SELECT statement
- [x] 1.4 Implement `UpsertIndexCache(db *sql.DB, rows []IndexCacheRow) error` using INSERT OR REPLACE
- [x] 1.5 Implement `DeleteIndexCache(db *sql.DB, filePaths []string) error` using a prepared DELETE statement
- [x] 1.6 Verify all store tests pass; commit

## 2. NoteEntry Struct and Index Core

- [x] 2.1 Write failing tests for `NoteEntry` struct fields and JSON round-trip serialization
- [x] 2.2 Define `NoteEntry` struct in `internal/index` with all fields from the spec
- [x] 2.3 Write failing tests for `Index.Get`, `Index.Update`, `Index.Delete`, `Index.All` with concurrent access
- [x] 2.4 Implement `Index` struct with `sync.RWMutex` and `map[string]*NoteEntry`
- [x] 2.5 Implement `Get`, `Update`, `Delete`, `All` methods with proper locking
- [x] 2.6 Verify tests pass; commit

## 3. Startup Scan and Cache Bootstrap

- [x] 3.1 Write failing tests for `Index.Build`: cache hit skips re-parse, cache miss triggers parse, deleted file is removed
- [x] 3.2 Implement `Index.Build(ctx context.Context, workspaceRoot string, db *sql.DB) error` — stat walk, mtime comparison, parse-or-load logic
- [x] 3.3 Implement dirty-flag setting on cache misses within `Build`
- [x] 3.4 Implement removal of stale cache entries for files no longer on disk (call `DeleteIndexCache`)
- [x] 3.5 Verify all build/scan tests pass; commit

## 4. BacklinkCount Two-Pass Computation

- [x] 4.1 Write failing tests for backlink count accuracy after `Build` on a fixture set of notes with known link relationships
- [x] 4.2 Implement two-pass backlink computation inside `Build`: first pass loads all entries, second pass counts backlinks by matching paths and aliases
- [x] 4.3 Verify backlink tests pass; commit

## 5. Dirty Tracking and FlushDirty

- [x] 5.1 Write failing tests for `Index.FlushDirty`: only dirty entries written, clean entries skipped, dirty flags cleared after flush
- [x] 5.2 Implement `dirty bool` flag (unexported) on `NoteEntry`; set in `Update`
- [x] 5.3 Implement `Index.FlushDirty(db *sql.DB) error` — collect dirty entries, call `UpsertIndexCache`, clear flags
- [x] 5.4 Verify flush tests pass; commit
