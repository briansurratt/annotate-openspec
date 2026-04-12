package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// IndexCacheRow represents a single row in the index_cache table.
// It holds the serialized NoteEntry data alongside the mtime and hash
// used to detect staleness on startup.
type IndexCacheRow struct {
	FilePath string
	Mtime    time.Time
	Hash     string
	Data     string // JSON-encoded NoteEntry fields
}

const (
	queryLoadIndexCache = `SELECT file_path, mtime, hash, data FROM index_cache`

	queryUpsertIndexCache = `
INSERT OR REPLACE INTO index_cache (file_path, mtime, hash, data)
VALUES (?, ?, ?, ?)`
)

// LoadIndexCache bulk-loads all rows from the index_cache table.
// It returns an empty (non-nil) slice when the table is empty.
func LoadIndexCache(db *sql.DB) ([]IndexCacheRow, error) {
	rows, err := db.Query(queryLoadIndexCache)
	if err != nil {
		return nil, fmt.Errorf("load index cache: %w", err)
	}
	defer rows.Close()

	var result []IndexCacheRow
	for rows.Next() {
		var r IndexCacheRow
		var mtimeStr string
		if err := rows.Scan(&r.FilePath, &mtimeStr, &r.Hash, &r.Data); err != nil {
			return nil, fmt.Errorf("scan index cache row: %w", err)
		}
		r.Mtime, err = time.Parse(time.RFC3339Nano, mtimeStr)
		if err != nil {
			// Fallback for rows written without nanosecond precision.
			r.Mtime, err = time.Parse(time.RFC3339, mtimeStr)
			if err != nil {
				return nil, fmt.Errorf("parse mtime %q: %w", mtimeStr, err)
			}
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index cache rows: %w", err)
	}
	if result == nil {
		result = []IndexCacheRow{}
	}
	return result, nil
}

// UpsertIndexCache writes or replaces index cache entries.
// Each row is inserted or replaced by file_path (the primary key).
// An empty slice is a no-op.
func UpsertIndexCache(db *sql.DB, rows []IndexCacheRow) error {
	if len(rows) == 0 {
		return nil
	}

	stmt, err := db.Prepare(queryUpsertIndexCache)
	if err != nil {
		return fmt.Errorf("prepare upsert index cache: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		mtimeStr := r.Mtime.UTC().Format(time.RFC3339Nano)
		if _, err := stmt.Exec(r.FilePath, mtimeStr, r.Hash, r.Data); err != nil {
			return fmt.Errorf("upsert index cache row %q: %w", r.FilePath, err)
		}
	}
	return nil
}

// DeleteIndexCache removes stale index cache entries for files that no longer
// exist on disk. An empty slice is a no-op.
func DeleteIndexCache(db *sql.DB, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// Build a parameterized DELETE with one placeholder per path.
	placeholders := strings.Repeat("?,", len(filePaths))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

	query := fmt.Sprintf(`DELETE FROM index_cache WHERE file_path IN (%s)`, placeholders)

	args := make([]any, len(filePaths))
	for i, p := range filePaths {
		args[i] = p
	}

	if _, err := db.Exec(query, args...); err != nil {
		return fmt.Errorf("delete index cache entries: %w", err)
	}
	return nil
}
