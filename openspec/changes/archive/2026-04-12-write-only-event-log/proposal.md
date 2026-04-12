## Why

The daemon needs an observable audit trail of every significant file-lifecycle event so that operators can diagnose annotation failures, measure throughput, and reason about retry behavior. Without a durable event log, failures are silent and there is no basis for the metrics reported in M5.

## What Changes

- Introduce `internal/eventlog` — a write-only package that appends structured rows to the `event_log` SQLite table.
- Support all six event types required by the M1 milestone: `saved`, `enqueued`, `skipped_excluded_namespace`, `conflict_discarded`, `retry_needed`, `processed`.
- Provide configurable retention so the table does not grow unbounded; the retention window is read from the config subsystem.
- No read path is exposed in this change — queries against the event log are deferred to the metrics/reporting milestone (M5).

## Capabilities

### New Capabilities
- `event-log`: Write-only append to the SQLite `event_log` table; supports all six event types, carries a per-event JSON `details` payload, and enforces a configurable retention policy via periodic row deletion.

### Modified Capabilities
- `database-store`: The `event_log` table schema must be fully specified (column names, types, constraints, and index) to match the eventlog package's INSERT statement.

## Impact

- **New package**: `internal/eventlog` — depends on `internal/store` (for `*sql.DB`) and `internal/config` (for retention window).
- **Modified spec**: `openspec/specs/database-store/spec.md` — the existing event_log table requirement is under-specified; this change tightens the schema definition.
- **No API surface change** — the eventlog package is consumed internally; no CLI commands or HTTP endpoints are added here.
