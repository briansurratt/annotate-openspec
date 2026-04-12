package queue

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/briansurratt/annotate/internal/store"
)

// openTestDB opens a fully-migrated SQLite database in a temp directory and
// returns the underlying *sql.DB. The caller is responsible for closing the store.
func openTestDB(t *testing.T) (*store.Store, func()) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return s, func() { s.Close() }
}

// TestNew_ReturnsQueueOnValidDB asserts that New succeeds with a valid, migrated DB.
func TestNew_ReturnsQueueOnValidDB(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if q == nil {
		t.Fatal("New() returned nil Queue")
	}
	defer q.Close()
}

// TestNew_ReturnsErrorOnNilDB asserts that New rejects a nil *sql.DB.
func TestNew_ReturnsErrorOnNilDB(t *testing.T) {
	q, err := New(nil)
	if err == nil {
		t.Error("New(nil) expected error, got nil")
	}
	if q != nil {
		t.Error("New(nil) expected nil Queue on error")
	}
}

// rowCount returns the number of rows in the queue table with the given status.
func rowCount(t *testing.T, s *store.Store, status string) int {
	t.Helper()
	var n int
	err := s.DB().QueryRow(`SELECT COUNT(*) FROM queue WHERE status = ?`, status).Scan(&n)
	if err != nil {
		t.Fatalf("rowCount: %v", err)
	}
	return n
}

// mtimeFor returns the mtime stored for the given file_path in queue.
func mtimeFor(t *testing.T, s *store.Store, path string) int64 {
	t.Helper()
	var m int64
	err := s.DB().QueryRow(`SELECT mtime FROM queue WHERE file_path = ?`, path).Scan(&m)
	if err != nil {
		t.Fatalf("mtimeFor %q: %v", path, err)
	}
	return m
}

// TestEnqueue_NewPath asserts that enqueueing a new path inserts exactly one row.
func TestEnqueue_NewPath(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Enqueue("/notes/a.md", 1000); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if n := rowCount(t, s, "pending"); n != 1 {
		t.Errorf("expected 1 pending row, got %d", n)
	}
	if m := mtimeFor(t, s, "/notes/a.md"); m != 1000 {
		t.Errorf("expected mtime=1000, got %d", m)
	}
}

// TestEnqueue_DuplicatePending asserts that re-enqueueing a pending path updates
// the mtime in place without inserting a second row.
func TestEnqueue_DuplicatePending(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Enqueue("/notes/a.md", 1000); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	if err := q.Enqueue("/notes/a.md", 2000); err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}

	if n := rowCount(t, s, "pending"); n != 1 {
		t.Errorf("expected 1 pending row after dedup, got %d", n)
	}
	if m := mtimeFor(t, s, "/notes/a.md"); m != 2000 {
		t.Errorf("expected updated mtime=2000, got %d", m)
	}
}

// TestDequeue_ReturnsFrontEntryAndMarksProcessing asserts that Dequeue returns
// the oldest pending entry and updates its status to 'processing'.
func TestDequeue_ReturnsFrontEntryAndMarksProcessing(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Enqueue("/notes/a.md", 1000); err != nil {
		t.Fatalf("Enqueue a: %v", err)
	}
	if err := q.Enqueue("/notes/b.md", 2000); err != nil {
		t.Fatalf("Enqueue b: %v", err)
	}

	entry, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if entry == nil {
		t.Fatal("Dequeue returned nil entry, expected /notes/a.md")
	}
	if entry.FilePath != "/notes/a.md" {
		t.Errorf("expected FilePath=/notes/a.md, got %q", entry.FilePath)
	}
	if entry.Mtime != 1000 {
		t.Errorf("expected Mtime=1000, got %d", entry.Mtime)
	}

	// The dequeued entry must now be 'processing'.
	if n := rowCount(t, s, "processing"); n != 1 {
		t.Errorf("expected 1 processing row, got %d", n)
	}
	// The second entry must still be pending.
	if n := rowCount(t, s, "pending"); n != 1 {
		t.Errorf("expected 1 pending row, got %d", n)
	}
}

// TestDequeue_EmptyQueueReturnsNil asserts that Dequeue returns nil, nil when
// no pending rows exist.
func TestDequeue_EmptyQueueReturnsNil(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	entry, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue on empty queue error: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry on empty queue, got %+v", entry)
	}
}

// TestReEnqueue_MovesProcessingToPendingAtBack asserts that ReEnqueue sets the
// row back to 'pending' and places it after all other rows.
func TestReEnqueue_MovesProcessingToPendingAtBack(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	// Enqueue two paths then dequeue the first.
	if err := q.Enqueue("/notes/a.md", 1000); err != nil {
		t.Fatalf("Enqueue a: %v", err)
	}
	if err := q.Enqueue("/notes/b.md", 2000); err != nil {
		t.Fatalf("Enqueue b: %v", err)
	}
	entry, err := q.Dequeue()
	if err != nil || entry == nil {
		t.Fatalf("Dequeue: err=%v entry=%v", err, entry)
	}

	// Re-enqueue the processing entry.
	if err := q.ReEnqueue(entry.ID); err != nil {
		t.Fatalf("ReEnqueue: %v", err)
	}

	// Both rows should now be pending.
	if n := rowCount(t, s, "pending"); n != 2 {
		t.Errorf("expected 2 pending rows, got %d", n)
	}
	if n := rowCount(t, s, "processing"); n != 0 {
		t.Errorf("expected 0 processing rows, got %d", n)
	}

	// The re-enqueued entry should come back last (after /notes/b.md).
	first, err := q.Dequeue()
	if err != nil || first == nil {
		t.Fatalf("first Dequeue after re-enqueue: err=%v entry=%v", err, first)
	}
	if first.FilePath != "/notes/b.md" {
		t.Errorf("expected /notes/b.md first, got %q", first.FilePath)
	}
}

// TestReEnqueue_NonExistentIDReturnsError asserts that ReEnqueue returns an
// error when no row with the given ID exists.
func TestReEnqueue_NonExistentIDReturnsError(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.ReEnqueue(999); err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

// TestEnqueue_ProcessingPathInsertsNewRow asserts that enqueueing a path whose
// existing row is 'processing' inserts a fresh 'pending' row alongside it.
func TestEnqueue_ProcessingPathInsertsNewRow(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	// Insert a row directly with status='processing' to simulate a dequeued entry.
	_, dbErr := s.DB().Exec(
		`INSERT INTO queue (file_path, mtime, position, status, created_at, updated_at)
		 VALUES (?, ?, 1, 'processing', 0, 0)`,
		"/notes/a.md", 1000,
	)
	if dbErr != nil {
		t.Fatalf("seed processing row: %v", dbErr)
	}

	if err := q.Enqueue("/notes/a.md", 2000); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Expect one 'processing' row and one 'pending' row.
	if n := rowCount(t, s, "processing"); n != 1 {
		t.Errorf("expected 1 processing row, got %d", n)
	}
	if n := rowCount(t, s, "pending"); n != 1 {
		t.Errorf("expected 1 pending row, got %d", n)
	}
}
