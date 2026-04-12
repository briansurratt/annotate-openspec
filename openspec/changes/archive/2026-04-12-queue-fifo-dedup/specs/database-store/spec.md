## MODIFIED Requirements

### Requirement: Queue Table Schema
The queue table SHALL have columns for id, file_path, mtime, position, status, created_at, updated_at.

#### Scenario: Queue table structure
- **WHEN** migrations are applied
- **THEN** queue table exists with columns: `id INTEGER PRIMARY KEY`, `file_path TEXT NOT NULL`, `mtime INTEGER NOT NULL`, `position INTEGER NOT NULL`, `status TEXT NOT NULL DEFAULT 'pending'`, `created_at INTEGER NOT NULL`, `updated_at INTEGER NOT NULL`
- **AND** a UNIQUE index is created on `(file_path, status)` filtered to `status = 'pending'` to enforce deduplication
- **AND** an index is created on `(status, position)` for efficient FIFO dequeue queries
