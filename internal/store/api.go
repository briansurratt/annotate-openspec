package store

import (
	"context"
	"fmt"
	"time"
)

// SchemaVersion describes an applied migration.
type SchemaVersion struct {
	Version   int
	Name      string
	AppliedAt time.Time
}

// SchemaVersions returns all applied migrations in version order.
func (s *Store) SchemaVersions(ctx context.Context) ([]SchemaVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT version, name, applied_at FROM schema_versions ORDER BY version`,
	)
	if err != nil {
		return nil, fmt.Errorf("query schema versions: %w", err)
	}
	defer rows.Close()

	var versions []SchemaVersion
	for rows.Next() {
		var sv SchemaVersion
		var appliedAt string
		if err := rows.Scan(&sv.Version, &sv.Name, &appliedAt); err != nil {
			return nil, fmt.Errorf("scan schema version: %w", err)
		}
		sv.AppliedAt, err = time.Parse("2006-01-02T15:04:05.000Z", appliedAt)
		if err != nil {
			// Fallback to RFC3339 if millisecond format fails.
			sv.AppliedAt, err = time.Parse(time.RFC3339, appliedAt)
			if err != nil {
				return nil, fmt.Errorf("parse applied_at timestamp %q: %w", appliedAt, err)
			}
		}
		versions = append(versions, sv)
	}
	return versions, rows.Err()
}

// Ping verifies the database connection is still alive.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}
