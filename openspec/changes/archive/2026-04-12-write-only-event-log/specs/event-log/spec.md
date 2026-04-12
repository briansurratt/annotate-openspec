## ADDED Requirements

### Requirement: Event Type Constants
The eventlog package SHALL define a Go string type `EventType` and constants for all six event types used in M1: `saved`, `enqueued`, `skipped_excluded_namespace`, `conflict_discarded`, `retry_needed`, `processed`.

#### Scenario: Constants are exported
- **WHEN** any package imports `internal/eventlog`
- **THEN** all six `EventType` constants are accessible without casting raw strings

### Requirement: Append Event
The eventlog package SHALL expose `Append(db *sql.DB, eventType EventType, filePath string, details map[string]any) error` that inserts one row into the `event_log` table with a UTC nanosecond timestamp.

#### Scenario: Successful append
- **WHEN** `Append` is called with a valid `*sql.DB`, a recognized event type, a non-empty file path, and a non-nil details map
- **THEN** exactly one row is inserted into `event_log`
- **AND** the row's `event_type` matches the argument
- **AND** the row's `file_path` matches the argument
- **AND** the row's `details` column contains valid JSON encoding of the map
- **AND** the row's `timestamp` is within 1 second of the current UTC time
- **AND** nil is returned

#### Scenario: Append with nil details
- **WHEN** `Append` is called with `details` set to `nil`
- **THEN** the row is inserted with `details` stored as `'{}'` (empty JSON object)
- **AND** nil is returned

#### Scenario: Database error propagation
- **WHEN** `Append` is called and the database write fails
- **THEN** an error wrapping the underlying database error is returned
- **AND** no partial row remains in the table

#### Scenario: Concurrent appends do not lose rows
- **WHEN** 10 goroutines each call `Append` simultaneously for distinct file paths
- **THEN** all 10 rows are present in `event_log` after all goroutines complete
- **AND** no error is returned by any goroutine

### Requirement: Prune Old Events
The eventlog package SHALL expose `Prune(db *sql.DB, retentionDays int) error` that deletes all rows from `event_log` whose `timestamp` is older than `now - retentionDays * 24h`.

#### Scenario: Rows outside retention window are deleted
- **WHEN** `Prune` is called with `retentionDays = 7`
- **THEN** all rows with `timestamp` older than 7 days are deleted
- **AND** rows within the 7-day window are not deleted
- **AND** nil is returned

#### Scenario: Zero retention deletes all rows
- **WHEN** `Prune` is called with `retentionDays = 0`
- **THEN** all rows in the table are deleted
- **AND** nil is returned

#### Scenario: Prune on empty table
- **WHEN** `Prune` is called on a table with no rows
- **THEN** nil is returned and no error occurs
