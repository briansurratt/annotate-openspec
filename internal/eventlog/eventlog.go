// Package eventlog provides a write-only append layer for the event_log SQLite
// table. It records all significant file-lifecycle events emitted by the daemon,
// queue worker, and fsWatcher goroutines. No read path is exposed; queries are
// handled by the metrics/reporting subsystem (M5).
package eventlog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventType identifies the kind of file-lifecycle event being recorded.
type EventType string

const (
	// EventSaved is emitted when fsnotify detects a file write.
	EventSaved EventType = "saved"
	// EventEnqueued is emitted when a file is successfully added to the queue.
	EventEnqueued EventType = "enqueued"
	// EventSkippedExcludedNamespace is emitted when a file's namespace matches
	// an exclude glob in the config and processing is skipped.
	EventSkippedExcludedNamespace EventType = "skipped_excluded_namespace"
	// EventConflictDiscarded is emitted when a processing result is discarded
	// because the file was modified after it was dequeued.
	EventConflictDiscarded EventType = "conflict_discarded"
	// EventRetryNeeded is emitted when a transient error requires the item to be
	// re-enqueued for another processing attempt.
	EventRetryNeeded EventType = "retry_needed"
	// EventProcessed is emitted when a file has been successfully annotated and
	// written back to disk.
	EventProcessed EventType = "processed"
)

const queryInsertEvent = `
INSERT INTO event_log (event_type, file_path, details, timestamp)
VALUES (?, ?, ?, ?)`

const queryPruneEvents = `
DELETE FROM event_log WHERE timestamp < ?`

// Append inserts one event row into the event_log table. details may be nil, in
// which case an empty JSON object is stored. The timestamp is set to the current
// UTC time as Unix nanoseconds. Append is safe to call concurrently from multiple
// goroutines; SQLite WAL mode serializes the writes.
func Append(db *sql.DB, eventType EventType, filePath string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("eventlog: marshal details: %w", err)
	}

	stmt, err := db.Prepare(queryInsertEvent)
	if err != nil {
		return fmt.Errorf("eventlog: prepare insert: %w", err)
	}
	defer stmt.Close()

	ts := time.Now().UTC().UnixNano()
	if _, err := stmt.Exec(string(eventType), filePath, string(detailsJSON), ts); err != nil {
		return fmt.Errorf("eventlog: insert event: %w", err)
	}
	return nil
}

// Prune deletes all event_log rows whose timestamp is older than
// now - retentionDays*24h. A retentionDays of 0 deletes all rows. Prune is
// intended to be called on daemon startup and periodically in the background.
func Prune(db *sql.DB, retentionDays int) error {
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixNano()

	stmt, err := db.Prepare(queryPruneEvents)
	if err != nil {
		return fmt.Errorf("eventlog: prepare prune: %w", err)
	}
	defer stmt.Close()

	if _, err := stmt.Exec(cutoff); err != nil {
		return fmt.Errorf("eventlog: prune events: %w", err)
	}
	return nil
}
