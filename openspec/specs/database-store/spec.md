### Requirement: Database Initialization
The system SHALL initialize a SQLite database connection with WAL mode enabled for concurrent access.

#### Scenario: Successful database initialization
- **WHEN** the store package is initialized
- **THEN** a SQLite database file is created at `.annotate/db.sqlite`
- **AND** WAL mode is enabled
- **AND** a database connection is returned

### Requirement: Schema Migrations
The system SHALL execute schema migrations to create the required tables: queue, event_log, index_cache, metrics.

#### Scenario: Migration execution on startup
- **WHEN** the database is initialized
- **THEN** migration scripts are executed in version order
- **AND** all four tables are created with proper schemas
- **AND** indexes are created for performance

### Requirement: Queue Table Schema
The queue table SHALL have columns for id, file_path, mtime, position, status, created_at, updated_at.

#### Scenario: Queue table structure
- **WHEN** migrations are applied
- **THEN** queue table exists with columns: `id INTEGER PRIMARY KEY`, `file_path TEXT NOT NULL`, `mtime INTEGER NOT NULL`, `position INTEGER NOT NULL`, `status TEXT NOT NULL DEFAULT 'pending'`, `created_at INTEGER NOT NULL`, `updated_at INTEGER NOT NULL`
- **AND** a UNIQUE index is created on `(file_path, status)` filtered to `status = 'pending'` to enforce deduplication
- **AND** an index is created on `(status, position)` for efficient FIFO dequeue queries

### Requirement: Event Log Table Schema
The event_log table SHALL have fully typed columns: `id INTEGER PRIMARY KEY AUTOINCREMENT`, `event_type TEXT NOT NULL`, `file_path TEXT NOT NULL`, `details TEXT NOT NULL DEFAULT '{}'`, `timestamp INTEGER NOT NULL` (Unix nanoseconds, UTC).

#### Scenario: Event log table structure
- **WHEN** migrations are applied
- **THEN** `event_log` table exists with columns: `id INTEGER PRIMARY KEY AUTOINCREMENT`, `event_type TEXT NOT NULL`, `file_path TEXT NOT NULL`, `details TEXT NOT NULL DEFAULT '{}'`, `timestamp INTEGER NOT NULL`
- **AND** an index is created on `(timestamp)` to support efficient retention pruning

#### Scenario: Retention policy enforced by pruning
- **WHEN** `Prune` is called on the event log with a configured retention window
- **THEN** rows with `timestamp` older than `now - retention` are deleted
- **AND** rows within the retention window are preserved

### Requirement: Index Cache Table Schema
The index_cache table SHALL have columns for file_path, mtime, hash, data.

#### Scenario: Index cache table structure
- **WHEN** migrations are applied
- **THEN** index_cache table exists with required columns
- **AND** supports fast lookups by file_path

### Requirement: Metrics Table Schema
The metrics table SHALL have columns for id, metric_name, value, timestamp.

#### Scenario: Metrics table structure
- **WHEN** migrations are applied
- **THEN** metrics table exists with required columns
- **AND** supports time-series metric storage

### Requirement: Index cache read operations
The store package SHALL expose `LoadIndexCache(db *sql.DB) ([]IndexCacheRow, error)` to bulk-load all rows from the `index_cache` table on startup.

#### Scenario: Bulk load on startup
- **WHEN** `LoadIndexCache` is called on an initialized database
- **THEN** all rows from `index_cache` are returned as `IndexCacheRow` values containing `FilePath`, `Mtime`, `Hash`, and `Data` fields

#### Scenario: Empty cache returns empty slice
- **WHEN** `LoadIndexCache` is called on a database with no cache rows
- **THEN** an empty slice and nil error are returned

### Requirement: Index cache write operations
The store package SHALL expose `UpsertIndexCache(db *sql.DB, rows []IndexCacheRow) error` to write or update index cache entries using an INSERT OR REPLACE prepared statement.

#### Scenario: Upsert new entry
- **WHEN** `UpsertIndexCache` is called with a row whose `file_path` does not exist in the table
- **THEN** a new row is inserted

#### Scenario: Upsert existing entry
- **WHEN** `UpsertIndexCache` is called with a row whose `file_path` already exists
- **THEN** the existing row is replaced with the new values

### Requirement: Index cache delete operations
The store package SHALL expose `DeleteIndexCache(db *sql.DB, filePaths []string) error` to remove stale entries for files that no longer exist on disk.

#### Scenario: Delete stale entries
- **WHEN** `DeleteIndexCache` is called with a list of file paths
- **THEN** all matching rows are removed from `index_cache`
