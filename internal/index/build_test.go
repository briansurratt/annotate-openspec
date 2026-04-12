package index

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briansurratt/annotate/internal/store"
)

// openTestStore creates a temporary SQLite store for tests.
func openTestStore(t *testing.T) *sql.DB {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB()
}

// writeNote writes a minimal markdown note to path and returns its mtime.
func writeNote(t *testing.T, path, content string) time.Time {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat %s: %v", path, err)
	}
	return info.ModTime()
}

// ————————————————————————————————————————————————————
// Build: basic scan
// ————————————————————————————————————————————————————

func TestBuild_ParsesMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	writeNote(t, filepath.Join(dir, "a.md"), "---\ntitle: Note A\n---\n\nBody A\n")
	writeNote(t, filepath.Join(dir, "b.md"), "---\ntitle: Note B\n---\n\nBody B\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	all := idx.All()
	if len(all) != 2 {
		t.Errorf("All() = %d entries, want 2", len(all))
	}
}

func TestBuild_NonMarkdownIgnored(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	writeNote(t, filepath.Join(dir, "note.md"), "# Note\n")
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(idx.All()) != 1 {
		t.Errorf("All() = %d, want 1 (txt ignored)", len(idx.All()))
	}
}

// ————————————————————————————————————————————————————
// Build: cache hit / cache miss
// ————————————————————————————————————————————————————

func TestBuild_CacheHitLoadsFromCache(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "cached.md")
	mtime := writeNote(t, path, "---\ntitle: Cached\n---\n\nBody\n")

	// Pre-populate the cache with a known title to verify it comes from cache.
	cachedEntry := NoteEntry{
		Path:  path,
		Title: "From Cache",
		Mtime: mtime,
		Hash:  "cached-hash",
	}
	data, err := entryToJSON(&cachedEntry)
	if err != nil {
		t.Fatalf("entryToJSON: %v", err)
	}
	if err := store.UpsertIndexCache(db, []store.IndexCacheRow{
		{FilePath: path, Mtime: mtime, Hash: "cached-hash", Data: data},
	}); err != nil {
		t.Fatalf("UpsertIndexCache: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := idx.Get(path)
	if got == nil {
		t.Fatal("Get() = nil, want entry")
	}
	if got.Title != "From Cache" {
		t.Errorf("Title = %q, want %q (should come from cache)", got.Title, "From Cache")
	}
}

func TestBuild_CacheMissParseFile(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "new.md")
	writeNote(t, path, "---\ntitle: Parsed Title\n---\n\nBody\n")
	// No cache entry — triggers a parse.

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := idx.Get(path)
	if got == nil {
		t.Fatal("Get() = nil, want parsed entry")
	}
	if got.Title != "Parsed Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Parsed Title")
	}
}

func TestBuild_StaleCacheTriggersReparse(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "stale.md")
	writeNote(t, path, "---\ntitle: New Title\n---\n\nBody\n")
	info, _ := os.Stat(path)
	freshMtime := info.ModTime()

	// Seed the cache with an older mtime.
	oldMtime := freshMtime.Add(-time.Hour)
	if err := store.UpsertIndexCache(db, []store.IndexCacheRow{
		{FilePath: path, Mtime: oldMtime, Hash: "old-hash", Data: `{"path":"` + path + `","title":"Old Title","mtime":"` + oldMtime.Format(time.RFC3339) + `","hash":"old-hash"}`},
	}); err != nil {
		t.Fatalf("UpsertIndexCache: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := idx.Get(path)
	if got == nil {
		t.Fatal("Get() = nil")
	}
	if got.Title != "New Title" {
		t.Errorf("Title = %q, want %q (stale cache should be re-parsed)", got.Title, "New Title")
	}
}

// ————————————————————————————————————————————————————
// Build: stale cache entries deleted when file no longer exists
// ————————————————————————————————————————————————————

func TestBuild_DeletesStaleEntriesForMissingFiles(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	// Real file.
	realPath := filepath.Join(dir, "real.md")
	writeNote(t, realPath, "---\ntitle: Real\n---\n\nBody\n")

	// Ghost entry in the cache — file does not exist.
	ghostPath := filepath.Join(dir, "ghost.md")
	ghostMtime := time.Now().UTC()
	if err := store.UpsertIndexCache(db, []store.IndexCacheRow{
		{FilePath: ghostPath, Mtime: ghostMtime, Hash: "ghost", Data: `{"path":"` + ghostPath + `","title":"Ghost","mtime":"` + ghostMtime.Format(time.RFC3339) + `","hash":"ghost"}`},
	}); err != nil {
		t.Fatalf("UpsertIndexCache: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Ghost must not appear in the index.
	if got := idx.Get(ghostPath); got != nil {
		t.Errorf("ghost entry still in index: %v", got)
	}

	// Ghost must be removed from the SQLite cache.
	rows, err := store.LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache: %v", err)
	}
	for _, r := range rows {
		if r.FilePath == ghostPath {
			t.Errorf("ghost entry still in SQLite cache")
		}
	}
}

// ————————————————————————————————————————————————————
// Build: dirty flag set on cache misses
// ————————————————————————————————————————————————————

func TestBuild_CacheMissSetsDirty(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "dirty.md")
	writeNote(t, path, "---\ntitle: Dirty\n---\n\nBody\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// The entry was parsed (cache miss) — it should be dirty so FlushDirty
	// will persist it to SQLite.
	got := idx.Get(path)
	if got == nil {
		t.Fatal("Get() = nil")
	}
	if !idx.isDirty(path) {
		t.Error("cache-miss entry should be dirty after Build()")
	}
}

func TestBuild_CacheHitNotDirty(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "clean.md")
	mtime := writeNote(t, path, "---\ntitle: Clean\n---\n\nBody\n")

	entry := NoteEntry{Path: path, Title: "Clean", Mtime: mtime, Hash: "h"}
	data, _ := entryToJSON(&entry)
	if err := store.UpsertIndexCache(db, []store.IndexCacheRow{
		{FilePath: path, Mtime: mtime, Hash: "h", Data: data},
	}); err != nil {
		t.Fatalf("UpsertIndexCache: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if idx.isDirty(path) {
		t.Error("cache-hit entry should NOT be dirty after Build()")
	}
}
