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
