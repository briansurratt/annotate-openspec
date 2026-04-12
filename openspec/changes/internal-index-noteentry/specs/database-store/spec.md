## ADDED Requirements

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
