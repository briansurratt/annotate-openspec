package index

import (
	"context"
	"path/filepath"
	"testing"
)

// TestBuild_BacklinkCount_TwoPass verifies that BacklinkCount is accurately
// computed after Build on a fixture set of notes with known link relationships.
//
// Fixture:
//   - a.md  links to [[b]] and [[c]]
//   - b.md  links to [[a]]
//   - c.md  no outgoing links
//
// Expected BacklinkCounts after Build:
//   - a.md: 1  (linked from b)
//   - b.md: 1  (linked from a)
//   - c.md: 1  (linked from a)
func TestBuild_BacklinkCount_TwoPass(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	writeNote(t, filepath.Join(dir, "a.md"), "---\ntitle: Note A\n---\n\nSee [[b]] and [[c]].\n")
	writeNote(t, filepath.Join(dir, "b.md"), "---\ntitle: Note B\n---\n\nSee [[a]].\n")
	writeNote(t, filepath.Join(dir, "c.md"), "---\ntitle: Note C\n---\n\nNo outgoing links.\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cases := []struct {
		name          string
		path          string
		wantBacklinks int
	}{
		{"a.md linked from b", filepath.Join(dir, "a.md"), 1},
		{"b.md linked from a", filepath.Join(dir, "b.md"), 1},
		{"c.md linked from a", filepath.Join(dir, "c.md"), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.Get(tc.path)
			if got == nil {
				t.Fatalf("Get(%s) = nil", tc.path)
			}
			if got.BacklinkCount != tc.wantBacklinks {
				t.Errorf("BacklinkCount = %d, want %d", got.BacklinkCount, tc.wantBacklinks)
			}
		})
	}
}

func TestBuild_BacklinkCount_NoLinks(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	writeNote(t, filepath.Join(dir, "standalone.md"), "---\ntitle: Standalone\n---\n\nNo links here.\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := idx.Get(filepath.Join(dir, "standalone.md"))
	if got == nil {
		t.Fatal("Get() = nil")
	}
	if got.BacklinkCount != 0 {
		t.Errorf("BacklinkCount = %d, want 0", got.BacklinkCount)
	}
}

func TestBuild_BacklinkCount_AliasMatch(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	// target.md has alias "tgt"; source.md links to [[tgt]].
	writeNote(t, filepath.Join(dir, "target.md"), "---\ntitle: Target\naliases:\n  - tgt\n---\n\nI am the target.\n")
	writeNote(t, filepath.Join(dir, "source.md"), "---\ntitle: Source\n---\n\nLink to [[tgt]].\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	target := idx.Get(filepath.Join(dir, "target.md"))
	if target == nil {
		t.Fatal("Get(target.md) = nil")
	}
	if target.BacklinkCount != 1 {
		t.Errorf("BacklinkCount = %d, want 1 (alias match)", target.BacklinkCount)
	}
}

func TestBuild_BacklinkCount_TitleMatch(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	// source.md links to [[My Note]] which matches the title of target.md.
	writeNote(t, filepath.Join(dir, "target.md"), "---\ntitle: My Note\n---\n\nContent.\n")
	writeNote(t, filepath.Join(dir, "source.md"), "---\ntitle: Source\n---\n\nSee [[My Note]].\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	target := idx.Get(filepath.Join(dir, "target.md"))
	if target == nil {
		t.Fatal("Get(target.md) = nil")
	}
	if target.BacklinkCount != 1 {
		t.Errorf("BacklinkCount = %d, want 1 (title match)", target.BacklinkCount)
	}
}

func TestBuild_BacklinkCount_SelfLinkNotCounted(t *testing.T) {
	dir := t.TempDir()
	db := openTestStore(t)

	// self.md links to [[self]] — self-references should not count.
	writeNote(t, filepath.Join(dir, "self.md"), "---\ntitle: self\n---\n\nSee [[self]].\n")

	idx := New()
	if err := idx.Build(context.Background(), dir, db); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := idx.Get(filepath.Join(dir, "self.md"))
	if got == nil {
		t.Fatal("Get(self.md) = nil")
	}
	if got.BacklinkCount != 0 {
		t.Errorf("BacklinkCount = %d, want 0 (self-link excluded)", got.BacklinkCount)
	}
}
