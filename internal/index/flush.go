package index

import (
	"database/sql"
	"fmt"

	"github.com/briansurratt/annotate/internal/store"
)

// FlushDirty writes all dirty NoteEntry values to the SQLite index_cache table
// and clears their dirty flags. Clean entries are skipped entirely.
// This is called periodically by the indexFlusher goroutine and on graceful shutdown.
func (idx *Index) FlushDirty(db *sql.DB) error {
	idx.mu.Lock()
	dirty := make([]*NoteEntry, 0)
	for _, e := range idx.entries {
		if e.dirty {
			dirty = append(dirty, e)
		}
	}
	idx.mu.Unlock()

	if len(dirty) == 0 {
		return nil
	}

	rows := make([]store.IndexCacheRow, 0, len(dirty))
	for _, e := range dirty {
		data, err := entryToJSON(e)
		if err != nil {
			return fmt.Errorf("flush dirty: serialize %s: %w", e.Path, err)
		}
		rows = append(rows, store.IndexCacheRow{
			FilePath: e.Path,
			Mtime:    e.Mtime,
			Hash:     e.Hash,
			Data:     data,
		})
	}

	if err := store.UpsertIndexCache(db, rows); err != nil {
		return fmt.Errorf("flush dirty: upsert cache: %w", err)
	}

	// Clear dirty flags only after a successful write.
	idx.mu.Lock()
	for _, e := range dirty {
		e.dirty = false
	}
	idx.mu.Unlock()

	return nil
}
