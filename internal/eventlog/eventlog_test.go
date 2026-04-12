package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/briansurratt/annotate/internal/store"
)

// openTestDB opens a fully-migrated SQLite database in a temp directory.
// The caller must call the returned cleanup function.
func openTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return s.DB(), func() { s.Close() }
}

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant EventType
		want     string
	}{
		{"EventSaved", EventSaved, "saved"},
		{"EventEnqueued", EventEnqueued, "enqueued"},
		{"EventSkippedExcludedNamespace", EventSkippedExcludedNamespace, "skipped_excluded_namespace"},
		{"EventConflictDiscarded", EventConflictDiscarded, "conflict_discarded"},
		{"EventRetryNeeded", EventRetryNeeded, "retry_needed"},
		{"EventProcessed", EventProcessed, "processed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.want)
			}
		})
	}
}

// TestAppend_SuccessfulInsert verifies a well-formed call inserts exactly one row
// with the correct event_type, file_path, details JSON, and a recent timestamp.
func TestAppend_SuccessfulInsert(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	before := time.Now().UTC().UnixNano()
	details := map[string]any{"queue_position": 3}
	if err := Append(db, EventEnqueued, "/notes/foo.md", details); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	after := time.Now().UTC().UnixNano()

	var eventType, filePath, detailsStr string
	var ts int64
	err := db.QueryRow(
		`SELECT event_type, file_path, details, timestamp FROM event_log LIMIT 1`,
	).Scan(&eventType, &filePath, &detailsStr, &ts)
	if err != nil {
		t.Fatalf("query row: %v", err)
	}

	if eventType != string(EventEnqueued) {
		t.Errorf("event_type = %q, want %q", eventType, EventEnqueued)
	}
	if filePath != "/notes/foo.md" {
		t.Errorf("file_path = %q, want %q", filePath, "/notes/foo.md")
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(detailsStr), &got); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if got["queue_position"] != float64(3) {
		t.Errorf("details[queue_position] = %v, want 3", got["queue_position"])
	}

	if ts < before || ts > after {
		t.Errorf("timestamp %d not in [%d, %d]", ts, before, after)
	}
}

// TestAppend_NilDetailsStoresEmptyObject verifies nil details produces "{}".
func TestAppend_NilDetailsStoresEmptyObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	if err := Append(db, EventSaved, "/notes/bar.md", nil); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	var detailsStr string
	if err := db.QueryRow(`SELECT details FROM event_log LIMIT 1`).Scan(&detailsStr); err != nil {
		t.Fatalf("query details: %v", err)
	}
	if detailsStr != "{}" {
		t.Errorf("details = %q, want %q", detailsStr, "{}")
	}
}

// TestAppend_DatabaseErrorPropagates verifies a closed DB returns a wrapped error.
func TestAppend_DatabaseErrorPropagates(t *testing.T) {
	db, cleanup := openTestDB(t)
	cleanup() // close immediately to provoke an error

	err := Append(db, EventSaved, "/notes/err.md", nil)
	if err == nil {
		t.Fatal("Append() expected error on closed DB, got nil")
	}
}

// TestAppend_ConcurrentAppendsDoNotLoseRows verifies 10 concurrent callers each
// produce a row with no errors.
func TestAppend_ConcurrentAppendsDoNotLoseRows(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("/notes/concurrent_%02d.md", i)
			errs[i] = Append(db, EventSaved, path, nil)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Append() error = %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM event_log`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != n {
		t.Errorf("row count = %d, want %d", count, n)
	}
}

// TestPrune_DeletesRowsOutsideRetentionWindow verifies old rows are removed and
// recent rows are kept.
func TestPrune_DeletesRowsOutsideRetentionWindow(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	// Insert an old row (15 days ago).
	old := time.Now().UTC().Add(-15 * 24 * time.Hour).UnixNano()
	if _, err := db.Exec(
		`INSERT INTO event_log (event_type, file_path, details, timestamp) VALUES (?, ?, ?, ?)`,
		"saved", "/old.md", "{}", old,
	); err != nil {
		t.Fatalf("insert old row: %v", err)
	}

	// Insert a recent row (1 day ago).
	recent := time.Now().UTC().Add(-1 * 24 * time.Hour).UnixNano()
	if _, err := db.Exec(
		`INSERT INTO event_log (event_type, file_path, details, timestamp) VALUES (?, ?, ?, ?)`,
		"saved", "/recent.md", "{}", recent,
	); err != nil {
		t.Fatalf("insert recent row: %v", err)
	}

	if err := Prune(db, 7); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM event_log`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("rows after prune = %d, want 1 (recent row preserved)", count)
	}

	var path string
	if err := db.QueryRow(`SELECT file_path FROM event_log`).Scan(&path); err != nil {
		t.Fatalf("query remaining row: %v", err)
	}
	if path != "/recent.md" {
		t.Errorf("remaining row file_path = %q, want %q", path, "/recent.md")
	}
}

// TestPrune_ZeroRetentionDeletesAll verifies retentionDays=0 removes every row.
func TestPrune_ZeroRetentionDeletesAll(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	now := time.Now().UTC().UnixNano()
	for i := range 3 {
		if _, err := db.Exec(
			`INSERT INTO event_log (event_type, file_path, details, timestamp) VALUES (?, ?, ?, ?)`,
			"saved", fmt.Sprintf("/file_%d.md", i), "{}", now,
		); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	if err := Prune(db, 0); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM event_log`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("rows after Prune(0) = %d, want 0", count)
	}
}

// TestPrune_EmptyTableReturnsNil verifies Prune on an empty table does not error.
func TestPrune_EmptyTableReturnsNil(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	if err := Prune(db, 7); err != nil {
		t.Errorf("Prune() on empty table error = %v", err)
	}
}
