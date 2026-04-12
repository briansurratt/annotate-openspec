## Why

The annotation system requires a persistent database to store the processing queue, event logs, index cache, and metrics. This is foundational for the daemon's operation and data integrity, enabling reliable file processing and system monitoring.

## What Changes

- Implement `internal/store` package with SQLite database initialization
- Enable WAL mode for concurrent reads and writes
- Create schema migrations for four core tables: `queue`, `event_log`, `index_cache`, `metrics`
- Provide database connection management and migration execution

## Capabilities

### New Capabilities
- `database-store`: SQLite-based persistent storage with schema management for queue, events, index, and metrics

### Modified Capabilities
<!-- No existing capabilities are modified -->

## Impact

- New `internal/store` Go package
- SQLite dependency (`modernc.org/sqlite`)
- Database file created in workspace (`.annotate/db.sqlite`)
- Affects daemon startup and all data persistence operations