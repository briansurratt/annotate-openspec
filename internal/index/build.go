package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/briansurratt/annotate/internal/notes"
	"github.com/briansurratt/annotate/internal/store"
)

// Build populates the index by scanning all .md files in workspaceRoot.
// For each file it compares the on-disk mtime against the SQLite index_cache:
//   - Cache hit (mtime matches): deserialize the cached NoteEntry — no file read.
//   - Cache miss / stale: parse the file and mark the entry dirty.
//
// Cache rows for files that no longer exist on disk are deleted from SQLite.
// After loading all entries a second pass computes BacklinkCount.
func (idx *Index) Build(ctx context.Context, workspaceRoot string, db *sql.DB) error {
	// Load the full SQLite cache.
	cachedRows, err := store.LoadIndexCache(db)
	if err != nil {
		return fmt.Errorf("index build: load cache: %w", err)
	}
	cacheByPath := make(map[string]store.IndexCacheRow, len(cachedRows))
	for _, r := range cachedRows {
		cacheByPath[r.FilePath] = r
	}

	// Walk the workspace, collecting .md files.
	foundPaths := make(map[string]bool)
	err = filepath.WalkDir(workspaceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		mtime := info.ModTime()
		foundPaths[path] = true

		cached, inCache := cacheByPath[path]
		if inCache && !mtime.After(cached.Mtime) {
			// Cache hit: deserialize the stored entry without reading the file.
			entry, err := entryFromJSON(cached.Data)
			if err != nil {
				// Corrupt cache row — fall through to re-parse.
			} else {
				entry.dirty = false
				idx.mu.Lock()
				idx.entries[path] = entry
				idx.mu.Unlock()
				return nil
			}
		}

		// Cache miss or stale: parse the file.
		entry, err := parseEntry(path, info)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		entry.dirty = true
		idx.mu.Lock()
		idx.entries[path] = entry
		idx.mu.Unlock()
		return nil
	})
	if err != nil {
		return fmt.Errorf("index build: walk %s: %w", workspaceRoot, err)
	}

	// Delete stale cache rows for files that no longer exist.
	var stalePaths []string
	for path := range cacheByPath {
		if !foundPaths[path] {
			stalePaths = append(stalePaths, path)
		}
	}
	if len(stalePaths) > 0 {
		if err := store.DeleteIndexCache(db, stalePaths); err != nil {
			return fmt.Errorf("index build: delete stale cache: %w", err)
		}
	}

	// Second pass: compute BacklinkCount (done in build_backlinks.go).
	idx.computeBacklinks()

	return nil
}

// isDirty reports whether the entry at path is marked dirty.
// It is intentionally unexported; only tests and FlushDirty use it.
func (idx *Index) isDirty(path string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	e, ok := idx.entries[path]
	if !ok {
		return false
	}
	return e.dirty
}

// parseEntry reads and parses a markdown file, returning a populated NoteEntry.
func parseEntry(path string, mtime os.FileInfo) (*NoteEntry, error) {
	note, err := notes.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse note: %w", err)
	}

	entry := &NoteEntry{
		Path:  path,
		Mtime: mtime.ModTime(),
		Hash:  note.ContentHash(),
	}

	if v, ok := note.Frontmatter["title"]; ok {
		entry.Title, _ = v.(string)
	}
	if v, ok := note.Frontmatter["aliases"]; ok {
		entry.Aliases = toStringSlice(v)
	}
	if v, ok := note.Frontmatter["tags"]; ok {
		entry.Tags = toStringSlice(v)
	}

	entry.Headings = extractHeadings(note.Body)
	entry.Links = extractLinks(note.Body)

	return entry, nil
}

// toStringSlice converts an interface{} that is expected to be a YAML list
// of strings into a []string.
func toStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

var (
	headingRE = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	linkRE    = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
)

// extractHeadings returns all ATX heading text from body.
func extractHeadings(body string) []string {
	matches := headingRE.FindAllStringSubmatch(body, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		result = append(result, strings.TrimSpace(m[1]))
	}
	return result
}

// extractLinks returns all wiki-style [[link]] targets from body.
// For [[target|alias]] syntax, only the target part is returned.
func extractLinks(body string) []string {
	matches := linkRE.FindAllStringSubmatch(body, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		target := m[1]
		if i := strings.Index(target, "|"); i >= 0 {
			target = target[:i]
		}
		result = append(result, strings.TrimSpace(target))
	}
	return result
}

// entryToJSON serializes a NoteEntry to JSON (omitting the dirty flag).
func entryToJSON(e *NoteEntry) (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal NoteEntry: %w", err)
	}
	return string(b), nil
}

// entryFromJSON deserializes a NoteEntry from JSON.
func entryFromJSON(data string) (*NoteEntry, error) {
	var e NoteEntry
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return nil, fmt.Errorf("unmarshal NoteEntry: %w", err)
	}
	return &e, nil
}
