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
