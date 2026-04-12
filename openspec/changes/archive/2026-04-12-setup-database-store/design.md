## Context

The annotation system needs persistent storage for its core data structures: a processing queue, event logs for auditing, an index cache for performance, and metrics for monitoring. The `internal/store` package will provide SQLite-based storage with proper schema management and WAL mode for concurrent access.

## Goals / Non-Goals

**Goals:**
- Initialize SQLite database with WAL mode enabled
- Implement schema migrations for four tables: queue, event_log, index_cache, metrics
- Provide database connection management and migration execution
- Ensure data integrity and concurrent read/write access

**Non-Goals:**
- Implement business logic for queue operations, event logging, or metrics collection
- Handle database backups or replication
- Support multiple database backends (SQLite only)

## Decisions

- **Database**: SQLite with `modernc.org/sqlite` driver for pure Go implementation and WAL mode support
- **WAL Mode**: Enabled by default for concurrent readers and single writer
- **Migrations**: Versioned SQL migration files executed in order on startup
- **Connection**: Single connection with proper cleanup on shutdown
- **Schema**: Four core tables with indexes for performance

## Risks / Trade-offs

- **SQLite file locking**: WAL mode mitigates but single-writer limitation could impact high-concurrency scenarios → Monitor and consider connection pooling if needed
- **Migration failures**: Could leave database in inconsistent state → Implement rollback capability and backup before migrations
- **File system permissions**: Database file needs write access → Ensure proper directory permissions in `.annotate/` folder