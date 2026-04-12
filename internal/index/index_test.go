package index

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ————————————————————————————————————————————————————
// NoteEntry struct and JSON round-trip
// ————————————————————————————————————————————————————

func TestNoteEntry_Fields(t *testing.T) {
	mtime := time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC)
	e := NoteEntry{
		Path:          "/notes/foo.md",
		Title:         "Foo Note",
		Aliases:       []string{"foo", "the foo"},
		Tags:          []string{"project", "active"},
		Mtime:         mtime,
		Hash:          "deadbeef",
		Headings:      []string{"Overview", "Details"},
		Links:         []string{"/notes/bar.md"},
		BacklinkCount: 3,
	}

	if e.Path != "/notes/foo.md" {
		t.Errorf("Path = %q, want /notes/foo.md", e.Path)
	}
	if e.Title != "Foo Note" {
		t.Errorf("Title = %q, want Foo Note", e.Title)
	}
	if len(e.Aliases) != 2 {
		t.Errorf("len(Aliases) = %d, want 2", len(e.Aliases))
	}
	if len(e.Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(e.Tags))
	}
	if !e.Mtime.Equal(mtime) {
		t.Errorf("Mtime = %v, want %v", e.Mtime, mtime)
	}
	if e.Hash != "deadbeef" {
		t.Errorf("Hash = %q, want deadbeef", e.Hash)
	}
	if len(e.Headings) != 2 {
		t.Errorf("len(Headings) = %d, want 2", len(e.Headings))
	}
	if len(e.Links) != 1 {
		t.Errorf("len(Links) = %d, want 1", len(e.Links))
	}
	if e.BacklinkCount != 3 {
		t.Errorf("BacklinkCount = %d, want 3", e.BacklinkCount)
	}
}

func TestNoteEntry_JSONRoundTrip(t *testing.T) {
	mtime := time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC)
	original := NoteEntry{
		Path:          "/notes/foo.md",
		Title:         "Foo Note",
		Aliases:       []string{"foo"},
		Tags:          []string{"project"},
		Mtime:         mtime,
		Hash:          "deadbeef",
		Headings:      []string{"Overview"},
		Links:         []string{"/notes/bar.md"},
		BacklinkCount: 2,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded NoteEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Path != original.Path {
		t.Errorf("Path round-trip: got %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title round-trip: got %q, want %q", decoded.Title, original.Title)
	}
	if len(decoded.Aliases) != len(original.Aliases) {
		t.Errorf("Aliases round-trip length: got %d, want %d", len(decoded.Aliases), len(original.Aliases))
	}
	if len(decoded.Tags) != len(original.Tags) {
		t.Errorf("Tags round-trip length: got %d, want %d", len(decoded.Tags), len(original.Tags))
	}
	if !decoded.Mtime.Equal(original.Mtime) {
		t.Errorf("Mtime round-trip: got %v, want %v", decoded.Mtime, original.Mtime)
	}
	if decoded.Hash != original.Hash {
		t.Errorf("Hash round-trip: got %q, want %q", decoded.Hash, original.Hash)
	}
	if len(decoded.Headings) != len(original.Headings) {
		t.Errorf("Headings round-trip length: got %d, want %d", len(decoded.Headings), len(original.Headings))
	}
	if len(decoded.Links) != len(original.Links) {
		t.Errorf("Links round-trip length: got %d, want %d", len(decoded.Links), len(original.Links))
	}
	if decoded.BacklinkCount != original.BacklinkCount {
		t.Errorf("BacklinkCount round-trip: got %d, want %d", decoded.BacklinkCount, original.BacklinkCount)
	}
}

func TestNoteEntry_DirtyNotExported(t *testing.T) {
	// dirty field must be unexported: JSON output must NOT contain it.
	e := NoteEntry{Path: "/notes/x.md", Hash: "h"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := m["dirty"]; ok {
		t.Error("JSON output contains 'dirty' field — it must be unexported")
	}
}

// ————————————————————————————————————————————————————
// Index.Get, Update, Delete, All — basic correctness
// ————————————————————————————————————————————————————

func sampleEntry(path string) *NoteEntry {
	return &NoteEntry{
		Path:  path,
		Title: "Title for " + path,
		Mtime: time.Now().UTC(),
		Hash:  "hash-" + path,
	}
}

func TestIndex_GetMissing(t *testing.T) {
	idx := New()
	e := idx.Get("/notes/missing.md")
	if e != nil {
		t.Errorf("Get() missing key = %v, want nil", e)
	}
}

func TestIndex_UpdateAndGet(t *testing.T) {
	idx := New()
	entry := sampleEntry("/notes/a.md")
	idx.Update(entry)

	got := idx.Get("/notes/a.md")
	if got == nil {
		t.Fatal("Get() after Update() = nil, want entry")
	}
	if got.Path != entry.Path {
		t.Errorf("Path = %q, want %q", got.Path, entry.Path)
	}
}

func TestIndex_Delete(t *testing.T) {
	idx := New()
	idx.Update(sampleEntry("/notes/a.md"))
	idx.Delete("/notes/a.md")

	if got := idx.Get("/notes/a.md"); got != nil {
		t.Errorf("Get() after Delete() = %v, want nil", got)
	}
}

func TestIndex_All(t *testing.T) {
	idx := New()
	idx.Update(sampleEntry("/notes/a.md"))
	idx.Update(sampleEntry("/notes/b.md"))

	all := idx.All()
	if len(all) != 2 {
		t.Errorf("All() = %d entries, want 2", len(all))
	}
}

func TestIndex_AllReturnsCopy(t *testing.T) {
	// Modifying the returned slice must not affect the index.
	idx := New()
	idx.Update(sampleEntry("/notes/a.md"))

	all := idx.All()
	all[0].Title = "mutated"

	got := idx.Get("/notes/a.md")
	if got.Title == "mutated" {
		t.Error("All() returned a reference that affects internal state")
	}
}

// ————————————————————————————————————————————————————
// Concurrent access tests
// ————————————————————————————————————————————————————

func TestIndex_ConcurrentReads(t *testing.T) {
	idx := New()
	for i := 0; i < 100; i++ {
		idx.Update(sampleEntry("/notes/note.md"))
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Get("/notes/note.md")
			_ = idx.All()
		}()
	}
	wg.Wait() // race detector will catch any issues
}

func TestIndex_ConcurrentWrites(t *testing.T) {
	idx := New()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			path := "/notes/note.md"
			idx.Update(&NoteEntry{Path: path, Hash: "hash"})
		}(i)
	}
	wg.Wait()

	got := idx.Get("/notes/note.md")
	if got == nil {
		t.Error("Get() after concurrent writes = nil, want an entry")
	}
}
