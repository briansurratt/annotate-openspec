package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB()
}

func TestLoadIndexCache_EmptyReturnsEmptySlice(t *testing.T) {
	db := openTestDB(t)

	rows, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("LoadIndexCache() on empty db = %d rows, want 0", len(rows))
	}
}

func TestLoadIndexCache_ReturnsAllRows(t *testing.T) {
	db := openTestDB(t)

	mtime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	want := []IndexCacheRow{
		{FilePath: "/notes/a.md", Mtime: mtime, Hash: "hash1", Data: `{"title":"A"}`},
		{FilePath: "/notes/b.md", Mtime: mtime.Add(time.Hour), Hash: "hash2", Data: `{"title":"B"}`},
	}

	if err := UpsertIndexCache(db, want); err != nil {
		t.Fatalf("UpsertIndexCache() setup error = %v", err)
	}

	got, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("LoadIndexCache() = %d rows, want %d", len(got), len(want))
	}

	// Build a map by FilePath for order-independent comparison.
	byPath := make(map[string]IndexCacheRow, len(got))
	for _, r := range got {
		byPath[r.FilePath] = r
	}
	for _, w := range want {
		r, ok := byPath[w.FilePath]
		if !ok {
			t.Errorf("missing row for %s", w.FilePath)
			continue
		}
		if r.Hash != w.Hash {
			t.Errorf("row %s: Hash = %q, want %q", w.FilePath, r.Hash, w.Hash)
		}
		if r.Data != w.Data {
			t.Errorf("row %s: Data = %q, want %q", w.FilePath, r.Data, w.Data)
		}
		if !r.Mtime.Equal(w.Mtime) {
			t.Errorf("row %s: Mtime = %v, want %v", w.FilePath, r.Mtime, w.Mtime)
		}
	}
}

func TestUpsertIndexCache_InsertsNewRow(t *testing.T) {
	db := openTestDB(t)

	row := IndexCacheRow{
		FilePath: "/notes/new.md",
		Mtime:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Hash:     "abc123",
		Data:     `{"title":"New"}`,
	}
	if err := UpsertIndexCache(db, []IndexCacheRow{row}); err != nil {
		t.Fatalf("UpsertIndexCache() error = %v", err)
	}

	rows, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("after insert: len = %d, want 1", len(rows))
	}
	if rows[0].FilePath != row.FilePath {
		t.Errorf("FilePath = %q, want %q", rows[0].FilePath, row.FilePath)
	}
}

func TestUpsertIndexCache_ReplacesExistingRow(t *testing.T) {
	db := openTestDB(t)

	original := IndexCacheRow{
		FilePath: "/notes/existing.md",
		Mtime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Hash:     "oldhash",
		Data:     `{"title":"Old"}`,
	}
	if err := UpsertIndexCache(db, []IndexCacheRow{original}); err != nil {
		t.Fatalf("UpsertIndexCache() initial insert error = %v", err)
	}

	updated := IndexCacheRow{
		FilePath: "/notes/existing.md",
		Mtime:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Hash:     "newhash",
		Data:     `{"title":"Updated"}`,
	}
	if err := UpsertIndexCache(db, []IndexCacheRow{updated}); err != nil {
		t.Fatalf("UpsertIndexCache() update error = %v", err)
	}

	rows, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("after update: len = %d, want 1", len(rows))
	}
	if rows[0].Hash != "newhash" {
		t.Errorf("Hash = %q, want %q", rows[0].Hash, "newhash")
	}
	if rows[0].Data != `{"title":"Updated"}` {
		t.Errorf("Data = %q, want updated value", rows[0].Data)
	}
}

func TestUpsertIndexCache_EmptySliceIsNoOp(t *testing.T) {
	db := openTestDB(t)

	if err := UpsertIndexCache(db, []IndexCacheRow{}); err != nil {
		t.Errorf("UpsertIndexCache() empty slice error = %v", err)
	}
}

func TestDeleteIndexCache_RemovesMatchingRows(t *testing.T) {
	db := openTestDB(t)

	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []IndexCacheRow{
		{FilePath: "/notes/keep.md", Mtime: mtime, Hash: "h1", Data: "{}"},
		{FilePath: "/notes/delete.md", Mtime: mtime, Hash: "h2", Data: "{}"},
	}
	if err := UpsertIndexCache(db, rows); err != nil {
		t.Fatalf("UpsertIndexCache() setup error = %v", err)
	}

	if err := DeleteIndexCache(db, []string{"/notes/delete.md"}); err != nil {
		t.Fatalf("DeleteIndexCache() error = %v", err)
	}

	got, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after delete: len = %d, want 1", len(got))
	}
	if got[0].FilePath != "/notes/keep.md" {
		t.Errorf("remaining row = %q, want /notes/keep.md", got[0].FilePath)
	}
}

func TestDeleteIndexCache_EmptySliceIsNoOp(t *testing.T) {
	db := openTestDB(t)

	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := UpsertIndexCache(db, []IndexCacheRow{
		{FilePath: "/notes/a.md", Mtime: mtime, Hash: "h", Data: "{}"},
	}); err != nil {
		t.Fatalf("UpsertIndexCache() setup error = %v", err)
	}

	if err := DeleteIndexCache(db, []string{}); err != nil {
		t.Errorf("DeleteIndexCache() empty slice error = %v", err)
	}

	got, err := LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("after empty delete: len = %d, want 1", len(got))
	}
}
