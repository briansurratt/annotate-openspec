package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briansurratt/annotate/internal/store"
)

func TestFlushDirty_OnlyWritesDirtyEntries(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	// Create 3 notes; all will be cache misses → dirty after Build.
	for _, name := range []string{"x.md", "y.md", "z.md"} {
		writeNote(t, filepath.Join(dir, name), "---\ntitle: "+name+"\n---\n\nBody\n")
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := idx.FlushDirty(db); err != nil {
		t.Fatalf("FlushDirty() error = %v", err)
	}

	rows, err := store.LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("LoadIndexCache after flush = %d rows, want 3", len(rows))
	}
}

func TestFlushDirty_ClearsFlags(t *testing.T) {
	db := openTestStore(t)

	idx := New()
	path := "/notes/flag.md"
	idx.Update(&NoteEntry{Path: path, Hash: "h", Mtime: time.Now().UTC()})

	if !idx.isDirty(path) {
		t.Fatal("entry should be dirty after Update()")
	}

	if err := idx.FlushDirty(db); err != nil {
		t.Fatalf("FlushDirty() error = %v", err)
	}

	if idx.isDirty(path) {
		t.Error("entry should NOT be dirty after FlushDirty()")
	}
}

func TestFlushDirty_SkipsCleanEntries(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	path := filepath.Join(dir, "clean.md")
	mtime := writeNote(t, path, "---\ntitle: Clean\n---\n\nBody\n")

	// Pre-seed cache so Build gets a hit (entry stays clean).
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
		t.Fatal("cache-hit entry should not be dirty before flush test")
	}

	// Delete the cache row, then FlushDirty. Since the entry is clean, no row
	// should be written back.
	if err := store.DeleteIndexCache(db, []string{path}); err != nil {
		t.Fatalf("DeleteIndexCache: %v", err)
	}

	if err := idx.FlushDirty(db); err != nil {
		t.Fatalf("FlushDirty() error = %v", err)
	}

	rows, err := store.LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("FlushDirty should not write clean entries; got %d rows", len(rows))
	}
}

func TestFlushDirty_OnlyTwoDirtyInFiveEntryIndex(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	allNames := []string{"a.md", "b.md", "c.md", "d.md", "e.md"}
	cachedNames := []string{"a.md", "b.md", "c.md"} // these get cache hits

	for _, name := range allNames {
		writeNote(t, filepath.Join(dir, name), "---\ntitle: "+name+"\n---\n\nBody\n")
	}

	// Seed cache for the first 3 with current on-disk mtime.
	var cacheRows []store.IndexCacheRow
	for _, name := range cachedNames {
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("Stat %s: %v", p, err)
		}
		e := NoteEntry{Path: p, Title: name, Mtime: fi.ModTime(), Hash: "h-" + name}
		d, _ := entryToJSON(&e)
		cacheRows = append(cacheRows, store.IndexCacheRow{
			FilePath: p, Mtime: fi.ModTime(), Hash: "h-" + name, Data: d,
		})
	}
	if err := store.UpsertIndexCache(db, cacheRows); err != nil {
		t.Fatalf("UpsertIndexCache: %v", err)
	}

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify exactly 2 dirty entries (d.md and e.md).
	dirtyCount := 0
	for _, name := range allNames {
		if idx.isDirty(filepath.Join(dir, name)) {
			dirtyCount++
		}
	}
	if dirtyCount != 2 {
		t.Errorf("dirty count = %d, want 2", dirtyCount)
	}

	// Wipe the cache, flush, and confirm only 2 rows appear.
	var allPaths []string
	for _, name := range allNames {
		allPaths = append(allPaths, filepath.Join(dir, name))
	}
	if err := store.DeleteIndexCache(db, allPaths); err != nil {
		t.Fatalf("DeleteIndexCache: %v", err)
	}

	if err := idx.FlushDirty(db); err != nil {
		t.Fatalf("FlushDirty() error = %v", err)
	}

	rows, err := store.LoadIndexCache(db)
	if err != nil {
		t.Fatalf("LoadIndexCache: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("after FlushDirty: %d rows, want 2 (only dirty entries flushed)", len(rows))
	}
}
