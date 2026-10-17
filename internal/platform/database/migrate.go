package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations is the set this binary knows how to apply.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		// Unreachable: the directory is embedded at build time.
		panic(err)
	}
	return sub
}

// ErrNewerDatabase reports a database migrated by a newer version of the
// server. Rolling the schema back automatically would be a good way to lose
// data, so this refuses instead.
var ErrNewerDatabase = errors.New("the database was migrated by a newer version of BookPlate")

// ErrMigrationChanged reports a migration whose content differs from the one
// that was applied. Editing an applied migration leaves every existing
// instance on a schema nobody can reproduce.
var ErrMigrationChanged = errors.New("an already applied migration has changed")

// Migrate applies every migration in fsys that has not run yet, in filename
// order, each in its own transaction. It returns the names it applied.
//
// fsys is a parameter rather than the embedded set so that tests can drive it
// with fixtures instead of the real schema.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	if err := ensureBookkeeping(ctx, db); err != nil {
		return nil, err
	}

	available, err := readMigrations(fsys)
	if err != nil {
		return nil, err
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(available))
	for _, m := range available {
		known[m.name] = true
	}
	for name := range applied {
		if !known[name] {
			return nil, fmt.Errorf("%w: it has applied %q, which this binary does not know",
				ErrNewerDatabase, name)
		}
	}

	var ran []string
	for _, m := range available {
		if checksum, ok := applied[m.name]; ok {
			if checksum != m.checksum {
				return nil, fmt.Errorf("%w: %q was applied as %s but is now %s",
					ErrMigrationChanged, m.name, checksum[:12], m.checksum[:12])
			}
			continue
		}
		if err := apply(ctx, db, m); err != nil {
			return ran, err
		}
		ran = append(ran, m.name)
	}
	return ran, nil
}

type migration struct {
	name     string
	sql      string
	checksum string
}

func readMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("listing migrations: %w", err)
	}
	sort.Strings(names)

	out := make([]migration, 0, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("reading migration %q: %w", name, err)
		}
		sum := sha256.Sum256(content)
		out = append(out, migration{
			name:     name,
			sql:      string(content),
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	return out, nil
}

func ensureBookkeeping(ctx context.Context, db *sql.DB) error {
	const stmt = `CREATE TABLE IF NOT EXISTS schema_migration (
		name       TEXT PRIMARY KEY,
		checksum   TEXT NOT NULL,
		applied_at TEXT NOT NULL
	) STRICT`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("creating schema_migration: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, checksum FROM schema_migration`)
	if err != nil {
		return nil, fmt.Errorf("reading schema_migration: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return nil, fmt.Errorf("reading schema_migration: %w", err)
		}
		out[name] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading schema_migration: %w", err)
	}
	return out, nil
}

// apply runs one migration and records it in the same transaction, so a
// half-applied migration cannot be recorded as done.
func apply(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %q: %w", m.name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("migration %q: %w", m.name, err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migration (name, checksum, applied_at) VALUES (?, ?, ?)`,
		m.name, m.checksum, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("recording migration %q: %w", m.name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %q: %w", m.name, err)
	}
	return nil
}
