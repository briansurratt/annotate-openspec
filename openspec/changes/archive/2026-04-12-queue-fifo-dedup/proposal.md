## Why

The daemon needs a durable work queue that survives restarts and prevents redundant AI calls when a file is saved multiple times in quick succession. Without a queue, the worker has no ordered, crash-safe place to draw jobs from.

## What Changes

- Introduce `internal/queue` package with a SQLite-backed FIFO deduplicating queue
- Enqueue a path: if the path is already in the queue, update its mtime and leave it at the back (dedup); otherwise append a new row
- Dequeue: pop the front row and return it to the caller
- Re-enqueue: push a path back onto the back of the queue (conflict path — the worker detected a concurrent file change and wants to retry)
- Remove: delete a successfully-processed row from the queue

## Capabilities

### New Capabilities
- `note-queue`: SQLite-backed FIFO deduplicating queue for note paths; supports enqueue (dedup), dequeue, re-enqueue, and remove operations

### Modified Capabilities
- `database-store`: the `queue` table definition and migration must live in `internal/store`; queue package depends on the existing `*sql.DB` setup

## Impact

- New package: `internal/queue`
- Depends on `internal/store` (SQLite `*sql.DB`, `queue` table already in migration)
- Used by: `internal/daemon` (fsWatcher goroutine calls Enqueue; queueWorker goroutine calls Dequeue/Remove/ReEnqueue)
- No new external dependencies — uses `database/sql` with the existing `modernc.org/sqlite` driver
