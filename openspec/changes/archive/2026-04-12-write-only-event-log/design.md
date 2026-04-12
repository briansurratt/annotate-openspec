## Context

The `internal/eventlog` package is one of the last foundational pieces of the M1 milestone. `internal/store` already provisions the `event_log` table (via migrations), and `internal/config` exposes a configurable retention window. This change wires them together into a thin, write-only append layer consumed by the daemon, queue worker, and fsWatcher goroutines.

The package must be safe to call concurrently from multiple goroutines (the four daemon goroutines all emit events) without external locking — SQLite WAL mode already provides the required write serialization at the database level.

## Goals / Non-Goals

**Goals:**
- Provide a single `Append(db, eventType, filePath, details)` call that inserts one row into `event_log`.
- Support all six event types used in M1: `saved`, `enqueued`, `skipped_excluded_namespace`, `conflict_discarded`, `retry_needed`, `processed`.
- Accept an optional JSON-serializable `details` map for per-event context (queue position, error message, etc.).
- Enforce retention via a `Prune(db, retentionDays int)` helper that deletes rows older than `now - retention`.
- Fully specify the `event_log` table schema (column names, types, NOT NULL constraints, index) in the `database-store` spec.

**Non-Goals:**
- No read / query path — that belongs to M5 metrics/reporting.
- No in-memory buffering or async flush — every `Append` call is a synchronous SQLite write; throughput is not a concern at M1 scale.
- No structured logger integration — the eventlog package is a data-persistence layer, not a logging framework.

## Decisions

### 1. `details` stored as JSON TEXT, not separate columns

**Decision**: serialize the per-event details map to a JSON string and store it in a single `details TEXT` column.

**Rationale**: The shape of details varies significantly per event type (e.g., `retry_needed` carries a retry count and error message; `processed` carries provider name and latency). A single JSON column avoids schema churn as new event types are added in later milestones. Querying specific fields inside `details` is deferred to M5, where appropriate indexes or views can be added if needed.

**Alternative considered**: one column per event type → rejected because it creates nullable sparse columns and makes migrations harder.

### 2. `Append` is synchronous with no connection pooling inside eventlog

**Decision**: `Append` and `Prune` accept `*sql.DB` as a parameter rather than owning a connection.

**Rationale**: The `*sql.DB` is initialized once in `internal/store` and passed to all subsystems. Owning a second connection in eventlog would complicate WAL checkpoint behavior and is unnecessary given SQLite's write serialization. Callers own lifecycle; eventlog owns semantics.

### 3. Retention enforced at prune time, not insert time

**Decision**: `Prune` is a separate function called on a configurable schedule (suggested: on daemon startup and periodically via the `indexFlusher` timer).

**Rationale**: Deleting old rows on every insert adds latency to the hot path. A background prune on a coarse schedule (e.g., every hour) is sufficient for a local single-user tool and avoids write amplification.

### 4. `event_log` schema tightens the existing `database-store` spec

The existing spec declares the table exists but does not specify column types or constraints. This change adds a delta requirement to `database-store` that pins the exact DDL used by `internal/eventlog`'s INSERT statement, preventing silent schema drift.

## Risks / Trade-offs

- **Goroutine contention on SQLite writes** → SQLite WAL mode serializes concurrent writes at the kernel level; individual `Append` calls are small (one row) so contention is negligible at M1 scale. Mitigated by monitoring `conflict_discards` in M6 if latency becomes an issue.
- **Details JSON silently drops non-serializable values** → `Append` uses `encoding/json.Marshal`; callers must pass only JSON-safe types (`string`, `int`, `bool`, `nil`). Enforced by code review and unit tests with fixture payloads.
- **No migration rollback** → adding a NOT NULL column to an existing table in SQLite requires a table rebuild. Since this is a new project with no prod data, this is not a concern. Future schema changes will follow the existing migration versioning in `internal/store`.
