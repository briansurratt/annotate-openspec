package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration represents a single versioned schema change.
type migration struct {
	version int
	name    string
	upSQL   string
	downSQL string // stored in a separate comment block at the top of the file
}

const createSchemaVersionsTable = `
CREATE TABLE IF NOT EXISTS schema_versions (
    version     INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    applied_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`

// runMigrations creates the schema_versions table if needed, then applies any
// unapplied migrations in version order.
func (s *Store) runMigrations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, createSchemaVersionsTable); err != nil {
		return fmt.Errorf("create schema_versions table: %w", err)
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		s.logger.Info("applying migration", "version", m.version, "name", m.name)
		if err := s.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %03d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// applyMigration runs a single migration inside a transaction.
func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on failure is intentional

	if _, err := tx.ExecContext(ctx, m.upSQL); err != nil {
		return fmt.Errorf("execute up SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_versions (version, name) VALUES (?, ?)`,
		m.version, m.name,
	); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	return tx.Commit()
}

// RollbackMigration rolls back the most recently applied migration. It is
// intended as a safety mechanism for emergency schema recovery and requires
// the migration file to include a -- down: block.
func (s *Store) RollbackMigration(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return err
	}

	// Find the highest applied version.
	latest := 0
	for v := range applied {
		if v > latest {
			latest = v
		}
	}
	if latest == 0 {
		return fmt.Errorf("no migrations to roll back")
	}

	var target migration
	for _, m := range migrations {
		if m.version == latest {
			target = m
			break
		}
	}
	if target.downSQL == "" {
		return fmt.Errorf("migration %03d has no rollback SQL", latest)
	}

	s.logger.Info("rolling back migration", "version", target.version, "name", target.name)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, target.downSQL); err != nil {
		return fmt.Errorf("execute down SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM schema_versions WHERE version = ?`, latest,
	); err != nil {
		return fmt.Errorf("remove migration record: %w", err)
	}

	return tx.Commit()
}

// appliedVersions returns the set of already-applied migration versions.
func (s *Store) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_versions`)
	if err != nil {
		return nil, fmt.Errorf("query applied versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// loadMigrations reads all *.sql files from the embedded migrations directory,
// parses version and name from the filename (e.g. 001_create_queue_table.sql),
// and returns them sorted by version.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		m, err := parseMigrationFile(e.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}

// parseMigrationFile reads a migration SQL file and extracts up/down SQL.
//
// File format:
//
//	-- down:
//	DROP TABLE IF EXISTS foo;
//	-- /down
//	CREATE TABLE foo (...);
//
// The -- down: / -- /down block is optional. Everything outside that block is
// treated as the "up" migration SQL.
func parseMigrationFile(filename string) (migration, error) {
	data, err := migrationsFS.ReadFile("migrations/" + filename)
	if err != nil {
		return migration{}, fmt.Errorf("read migration file %s: %w", filename, err)
	}

	var version int
	var name string
	if _, err := fmt.Sscanf(filename, "%03d_%s", &version, &name); err != nil {
		return migration{}, fmt.Errorf("parse migration filename %q: %w", filename, err)
	}
	name = strings.TrimSuffix(name, ".sql")

	content := string(data)
	upSQL, downSQL := splitUpDown(content)

	return migration{
		version: version,
		name:    name,
		upSQL:   upSQL,
		downSQL: downSQL,
	}, nil
}

// splitUpDown separates the -- down: block from the up SQL.
func splitUpDown(content string) (upSQL, downSQL string) {
	const downStart = "-- down:"
	const downEnd = "-- /down"

	startIdx := strings.Index(content, downStart)
	if startIdx == -1 {
		return strings.TrimSpace(content), ""
	}

	endIdx := strings.Index(content, downEnd)
	if endIdx == -1 {
		// Malformed — treat whole file as up SQL.
		return strings.TrimSpace(content), ""
	}

	// Extract down SQL (between markers).
	downBlock := content[startIdx+len(downStart) : endIdx]
	downSQL = strings.TrimSpace(downBlock)

	// Up SQL is everything after the down block.
	upSQL = strings.TrimSpace(content[endIdx+len(downEnd):])
	return upSQL, downSQL
}
