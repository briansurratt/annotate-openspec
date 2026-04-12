// Package store provides SQLite-backed persistent storage for the annotation daemon.
// It manages the database connection, WAL mode configuration, and schema migrations
// for the queue, event_log, index_cache, and metrics tables.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Register SQLite driver
)

// Store manages the SQLite database connection and schema.
type Store struct {
	db     *sql.DB
	dbPath string
	logger *slog.Logger
}

// Open initializes a SQLite database at dbPath, enables WAL mode, and runs
// any pending schema migrations. The caller must call Close when done.
func Open(ctx context.Context, dbPath string, logger *slog.Logger) (*Store, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer; WAL mode allows concurrent readers.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, dbPath: dbPath, logger: logger}

	if err := s.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}

	if err := s.runMigrations(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	logger.Info("database opened", "path", dbPath)
	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	s.logger.Info("database closed", "path", s.dbPath)
	return nil
}

// DB returns the underlying *sql.DB for use by other packages (queue, eventlog, etc.).
func (s *Store) DB() *sql.DB {
	return s.db
}

// configure sets SQLite pragmas required before any schema work.
func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := s.db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("set pragma %q: %w", p, err)
		}
	}
	return nil
}
