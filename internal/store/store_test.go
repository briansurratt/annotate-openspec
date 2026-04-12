package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".annotate", "db.sqlite")

	s, err := Open(context.Background(), dbPath, nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file was not created at %s", dbPath)
	}
}

func TestOpen_WALModeEnabled(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestRunMigrations_AllTablesCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	tables := []string{"queue", "event_log", "index_cache", "metrics"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			var name string
			err := s.db.QueryRowContext(context.Background(),
				`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
			).Scan(&name)
			if err != nil {
				t.Errorf("table %q not found: %v", table, err)
			}
		})
	}
}

func TestRunMigrations_IndexesCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	indexes := []string{
		"idx_queue_file_path",
		"idx_event_log_timestamp",
		"idx_metrics_name",
	}
	for _, idx := range indexes {
		t.Run(idx, func(t *testing.T) {
			var name string
			err := s.db.QueryRowContext(context.Background(),
				`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
			).Scan(&name)
			if err != nil {
				t.Errorf("index %q not found: %v", idx, err)
			}
		})
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	// Open twice to ensure migrations don't fail on re-run.
	for i := range 2 {
		s, err := Open(context.Background(), dbPath, nil)
		if err != nil {
			t.Fatalf("Open() pass %d error = %v", i+1, err)
		}
		s.Close()
	}
}

func TestSchemaVersions_RecordsAllMigrations(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	versions, err := s.SchemaVersions(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersions() error = %v", err)
	}

	// We ship 7 migration files; all should be recorded.
	const expectedCount = 7
	if len(versions) != expectedCount {
		t.Errorf("len(SchemaVersions()) = %d, want %d", len(versions), expectedCount)
	}

	// Versions must be strictly ascending.
	for i := 1; i < len(versions); i++ {
		if versions[i].Version <= versions[i-1].Version {
			t.Errorf("versions not ascending at index %d: %d <= %d",
				i, versions[i].Version, versions[i-1].Version)
		}
	}
}

func TestRollbackMigration_RemovesLatest(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	before, err := s.SchemaVersions(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersions() error = %v", err)
	}

	if err := s.RollbackMigration(context.Background()); err != nil {
		t.Fatalf("RollbackMigration() error = %v", err)
	}

	after, err := s.SchemaVersions(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersions() after rollback error = %v", err)
	}

	if len(after) != len(before)-1 {
		t.Errorf("after rollback: len(versions) = %d, want %d", len(after), len(before)-1)
	}
}

func TestPing(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	if err := s.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestQueueTableColumns(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	// INSERT a row to verify all required columns exist and accept values.
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO queue (file_path, mtime, status) VALUES (?, ?, ?)`,
		"/notes/foo.md", "2024-01-01T00:00:00Z", "pending",
	)
	if err != nil {
		t.Errorf("insert into queue: %v", err)
	}
}

func TestEventLogTableColumns(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO event_log (event_type, file_path, details) VALUES (?, ?, ?)`,
		"saved", "/notes/foo.md", "{}",
	)
	if err != nil {
		t.Errorf("insert into event_log: %v", err)
	}
}

func TestIndexCacheTableColumns(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO index_cache (file_path, mtime, hash, data) VALUES (?, ?, ?, ?)`,
		"/notes/foo.md", "2024-01-01T00:00:00Z", "abc123", "{}",
	)
	if err != nil {
		t.Errorf("insert into index_cache: %v", err)
	}
}

func TestMetricsTableColumns(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()

	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO metrics (metric_name, value) VALUES (?, ?)`,
		"files_processed", 42.0,
	)
	if err != nil {
		t.Errorf("insert into metrics: %v", err)
	}
}
