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

// TestRemove_DeletesExistingRow asserts that Remove deletes the row with the given ID.
func TestRemove_DeletesExistingRow(t *testing.T) {
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
	entry, err := q.Dequeue()
	if err != nil || entry == nil {
		t.Fatalf("Dequeue: err=%v entry=%v", err, entry)
	}

	if err := q.Remove(entry.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM queue`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows after remove, got %d", n)
	}
}

// TestRemove_NonExistentIDReturnsError asserts that Remove returns an error
// when no row with the given ID exists.
func TestRemove_NonExistentIDReturnsError(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Remove(999); err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

// --- Integration tests ---

// TestIntegration_FIFOCycle verifies the full FIFO lifecycle:
// enqueue A, enqueue B, dequeue (gets A), remove A, dequeue (gets B), remove B,
// then the queue is empty.
func TestIntegration_FIFOCycle(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Enqueue("/notes/a.md", 100); err != nil {
		t.Fatalf("Enqueue a: %v", err)
	}
	if err := q.Enqueue("/notes/b.md", 200); err != nil {
		t.Fatalf("Enqueue b: %v", err)
	}

	first, err := q.Dequeue()
	if err != nil || first == nil {
		t.Fatalf("first Dequeue: err=%v entry=%v", err, first)
	}
	if first.FilePath != "/notes/a.md" {
		t.Errorf("expected a.md first, got %q", first.FilePath)
	}
	if err := q.Remove(first.ID); err != nil {
		t.Fatalf("Remove a: %v", err)
	}

	second, err := q.Dequeue()
	if err != nil || second == nil {
		t.Fatalf("second Dequeue: err=%v entry=%v", err, second)
	}
	if second.FilePath != "/notes/b.md" {
		t.Errorf("expected b.md second, got %q", second.FilePath)
	}
	if err := q.Remove(second.ID); err != nil {
		t.Fatalf("Remove b: %v", err)
	}

	empty, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue on empty: %v", err)
	}
	if empty != nil {
		t.Errorf("expected nil on empty queue, got %+v", empty)
	}
}

// TestIntegration_Dedup verifies that enqueueing the same path twice results
// in a single row with the updated mtime, and dequeuing returns it exactly once.
func TestIntegration_Dedup(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	if err := q.Enqueue("/notes/x.md", 1000); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	if err := q.Enqueue("/notes/x.md", 2000); err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}

	var total int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM queue`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 row after dedup, got %d", total)
	}

	entry, err := q.Dequeue()
	if err != nil || entry == nil {
		t.Fatalf("Dequeue: err=%v entry=%v", err, entry)
	}
	if entry.Mtime != 2000 {
		t.Errorf("expected updated mtime=2000, got %d", entry.Mtime)
	}
	if err := q.Remove(entry.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Queue should be empty.
	empty, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue on empty: %v", err)
	}
	if empty != nil {
		t.Errorf("expected nil on empty queue, got %+v", empty)
	}
}

// TestIntegration_ConflictCycle verifies the conflict path:
// enqueue, dequeue (marks processing), re-enqueue (back of queue), dequeue again.
func TestIntegration_ConflictCycle(t *testing.T) {
	s, cleanup := openTestDB(t)
	defer cleanup()

	q, err := New(s.DB())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer q.Close()

	// Enqueue two paths so we can verify the re-enqueued entry ends up last.
	if err := q.Enqueue("/notes/a.md", 100); err != nil {
		t.Fatalf("Enqueue a: %v", err)
	}
	if err := q.Enqueue("/notes/b.md", 200); err != nil {
		t.Fatalf("Enqueue b: %v", err)
	}

	// Dequeue a — it becomes processing.
	entryA, err := q.Dequeue()
	if err != nil || entryA == nil {
		t.Fatalf("Dequeue a: err=%v entry=%v", err, entryA)
	}
	if entryA.FilePath != "/notes/a.md" {
		t.Errorf("expected a.md, got %q", entryA.FilePath)
	}

	// Re-enqueue a — pushes it to the back behind b.
	if err := q.ReEnqueue(entryA.ID); err != nil {
		t.Fatalf("ReEnqueue: %v", err)
	}

	// Dequeue again — should get b (the older pending entry).
	entryB, err := q.Dequeue()
	if err != nil || entryB == nil {
		t.Fatalf("second Dequeue: err=%v entry=%v", err, entryB)
	}
	if entryB.FilePath != "/notes/b.md" {
		t.Errorf("expected b.md next, got %q", entryB.FilePath)
	}

	// Dequeue once more — should get a (re-enqueued at back).
	entryA2, err := q.Dequeue()
	if err != nil || entryA2 == nil {
		t.Fatalf("third Dequeue: err=%v entry=%v", err, entryA2)
	}
	if entryA2.FilePath != "/notes/a.md" {
		t.Errorf("expected a.md at back, got %q", entryA2.FilePath)
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
