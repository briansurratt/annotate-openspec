## MODIFIED Requirements

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
