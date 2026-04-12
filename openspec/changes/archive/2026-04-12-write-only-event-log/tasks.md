## 1. Database Schema

- [x] 1.1 Add migration to `internal/store` that creates the `event_log` table with columns: `id INTEGER PRIMARY KEY AUTOINCREMENT`, `event_type TEXT NOT NULL`, `file_path TEXT NOT NULL`, `details TEXT NOT NULL DEFAULT '{}'`, `timestamp INTEGER NOT NULL`
- [x] 1.2 Add a `CREATE INDEX IF NOT EXISTS event_log_timestamp_idx ON event_log (timestamp)` to the same migration
- [x] 1.3 Write a migration integration test asserting all columns and the index exist after `InitDB`

## 2. EventType Constants

- [x] 2.1 Create `internal/eventlog/eventlog.go` with a `type EventType string` and constants for all six types: `EventSaved`, `EventEnqueued`, `EventSkippedExcludedNamespace`, `EventConflictDiscarded`, `EventRetryNeeded`, `EventProcessed`
- [x] 2.2 Write a unit test that asserts each constant's string value matches the spec (`saved`, `enqueued`, etc.)

## 3. Append Function

- [x] 3.1 Write failing tests for `Append` covering: successful insert, nil details defaults to `{}`, database error propagation, and concurrent appends (10 goroutines)
- [x] 3.2 Implement `Append(db *sql.DB, eventType EventType, filePath string, details map[string]any) error` using a prepared INSERT statement; serialize details with `encoding/json.Marshal`; use `time.Now().UTC().UnixNano()` for timestamp
- [x] 3.3 Verify all `Append` tests pass

## 4. Prune Function

- [x] 4.1 Write failing tests for `Prune` covering: rows outside retention deleted, rows inside retention preserved, zero retention deletes all, empty table returns nil
- [x] 4.2 Implement `Prune(db *sql.DB, retentionDays int) error` using a DELETE WHERE `timestamp < cutoff` prepared statement; cutoff = `time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixNano()`
- [x] 4.3 Verify all `Prune` tests pass

## 5. Commit

- [x] 5.1 Commit all changes with message: `feat(eventlog): implement write-only event log (M1.6)`
