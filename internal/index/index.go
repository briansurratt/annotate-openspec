// Package index maintains an in-memory map of NoteEntry values for all
// markdown files in the workspace. It provides a fast lookup layer for
// prompt assembly and is backed by an SQLite cache to avoid full re-parses
// on daemon restart.
package index

import (
	"sync"
	"time"
)

// NoteEntry represents a single note's indexed metadata.
// It holds only the fields needed for prompt assembly — never the body text.
// The dirty field (unexported) tracks whether the entry has been modified
// since the last flush to the SQLite index_cache table.
type NoteEntry struct {
	Path          string    `json:"path"`
	Title         string    `json:"title"`
	Aliases       []string  `json:"aliases,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	Mtime         time.Time `json:"mtime"`
	Hash          string    `json:"hash"`
	Headings      []string  `json:"headings,omitempty"`
	Links         []string  `json:"links,omitempty"`  // outgoing wiki-style links (paths/titles)
	BacklinkCount int       `json:"backlink_count"`

	dirty bool // not exported; not serialized
}

// Index is a thread-safe in-memory map of NoteEntry values keyed by
// absolute file path. A single RWMutex is used: reads take an RLock
// and writes take a full Lock.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*NoteEntry
}

// New returns an empty, ready-to-use Index.
func New() *Index {
	return &Index{
		entries: make(map[string]*NoteEntry),
	}
}

// Get returns the NoteEntry for path, or nil if the path is not indexed.
func (idx *Index) Get(path string) *NoteEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.entries[path]
}

// Update inserts or replaces the entry for entry.Path and marks it dirty.
// The entry pointer is stored directly; callers must not mutate it after
// calling Update.
func (idx *Index) Update(entry *NoteEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	entry.dirty = true
	idx.entries[entry.Path] = entry
}

// Delete removes the entry for path from the index.
func (idx *Index) Delete(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.entries, path)
}

// All returns a shallow copy of all NoteEntry pointers in the index.
// The returned slice is safe to iterate without holding the lock, but the
// NoteEntry values themselves are shared — callers must not modify them.
func (idx *Index) All() []*NoteEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := make([]*NoteEntry, 0, len(idx.entries))
	for _, e := range idx.entries {
		// Return a copy of each NoteEntry so callers cannot mutate internal state.
		cp := *e
		result = append(result, &cp)
	}
	return result
}
