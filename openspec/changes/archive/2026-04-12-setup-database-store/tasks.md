## 1. Package Setup

- [x] 1.1 Create internal/store package directory and init file
- [x] 1.2 Add modernc.org/sqlite dependency to go.mod

## 2. Database Connection

- [x] 2.1 Implement database initialization function with WAL mode
- [x] 2.2 Add connection management with proper cleanup

## 3. Schema Migrations

- [x] 3.1 Create migration system with version tracking
- [x] 3.2 Define SQL schemas for queue, event_log, index_cache, metrics tables
- [x] 3.3 Implement migration execution on startup
- [x] 3.4 Add migration rollback capability for safety

## 4. Table Schemas

- [x] 4.1 Implement queue table: id, file_path, mtime, status, created_at, updated_at
- [x] 4.2 Implement event_log table: id, event_type, file_path, details, timestamp
- [x] 4.3 Implement index_cache table: file_path, mtime, hash, data
- [x] 4.4 Implement metrics table: id, metric_name, value, timestamp
- [x] 4.5 Add appropriate indexes for performance

## 5. Integration

- [x] 5.1 Export public API for database operations
- [x] 5.2 Add error handling and logging
- [x] 5.3 Test database initialization and migrations
