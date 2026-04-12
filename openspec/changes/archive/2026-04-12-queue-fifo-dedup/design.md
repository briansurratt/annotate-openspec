## Context

The daemon's `fsWatcher` goroutine detects file saves and the `queueWorker` goroutine processes them. We need a shared, durable data structure that:
- Survives daemon restarts (jobs must not be lost)
- Deduplicates rapid saves of the same file (avoid redundant AI calls)
- Preserves FIFO order so the oldest pending change is processed first
- Supports re-queuing on conflict (worker detected a concurrent edit mid-processing)

The `queue` table already exists in `internal/store`'s migration — this change implements the Go interface that operates on it.

## Goals / Non-Goals

**Goals:**
- A `Queue` struct in `internal/queue` with `Enqueue`, `Dequeue`, `ReEnqueue`, and `Remove` methods
- Enqueue deduplicates: if a path is already pending, update its `mtime` in place (keep position at back via `updated_at` ordering) rather than inserting a duplicate row
- Dequeue returns the oldest pending row (FIFO) and marks it `processing`
- ReEnqueue moves a `processing` row back to `pending` at the back (conflict path)
- Remove deletes a row after successful processing
- All operations use prepared statements via the `*sql.DB` passed from `internal/store`

**Non-Goals:**
- Concurrent worker support — single worker assumption for M1; no row-level locking strategy needed yet
- Priority queuing — all entries are equal weight
- In-memory fallback — durability via SQLite is the requirement

## Decisions

### FIFO ordering via `position` column vs `created_at` timestamp

**Decision:** Use a monotonically incrementing `position` INTEGER (auto-assigned via SQLite `rowid` semantics on insert) rather than `created_at` for ordering.

**Rationale:** `created_at` timestamps can collide at sub-millisecond precision on fast saves; ROWID / explicit sequence is guaranteed unique and monotone. On dedup (update), we do NOT change `position` — the row stays where it was. `Dequeue` selects `WHERE status = 'pending' ORDER BY position ASC LIMIT 1`.

**Alternative considered:** Re-insert on dedup (delete + insert to move to back). Rejected — for FIFO semantics, we want to process the *oldest pending save* first; a re-insert would push a frequently-updated file to perpetual starvation. Updating `mtime` in place (so the worker sees the latest version when it finally dequeues) is the right behavior.

### Status column: `pending` / `processing`

**Decision:** Two status values only. A row exists until `Remove` is called; the worker marks it `processing` via `Dequeue` and calls `Remove` on success or `ReEnqueue` on conflict.

**Rationale:** Keeps state machine minimal for M1. `event_log` records all outcome events; the queue table only tracks "what still needs work".

### Prepared statements

All four methods prepare their SQL once at `Queue` construction time and reuse the `*sql.Stmt`. This avoids repeated parse overhead and is required by the coding standards ("use prepared statements").

## Risks / Trade-offs

- **Single-writer assumption** → If a second worker goroutine is added later, `Dequeue` will need a `BEGIN EXCLUSIVE` transaction to avoid double-dequeue. Acceptable for M1.
- **No WAL-mode enforcement in queue** → WAL is set at DB init in `internal/store`; `internal/queue` trusts that the `*sql.DB` it receives is already correctly configured.
- **Dedup updates `mtime` only** → If the queue is deep, the worker may process a stale `mtime` value. This is intentional: the worker re-reads the file from disk at processing time; the queued `mtime` is used only for conflict detection (compare with on-disk mtime at write time).
