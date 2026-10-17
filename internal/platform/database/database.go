// Package database opens the SQLite database with the settings a
// household-sized catalogue needs on modest hardware.
//
// The driver is pure Go, so the binary stays static and cross-compiles to
// arm64 without a C toolchain. See docs/adr/0003-sqlite-only.md.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// FileName is the database file inside the data directory. Keeping it fixed
// means an operator backs up one directory and is done.
const FileName = "bookplate.db"

// busyTimeout is how long a statement waits for a lock before failing. SQLite
// serialises writers, and a short wait turns a routine collision into a
// visible error for no reason.
const busyTimeout = "5000"

// Open opens, and creates if necessary, the database inside dir.
//
// The pragmas are not decoration:
//
//   - journal_mode(WAL) lets readers work while a writer holds the database,
//     which is what keeps a page responsive during a bulk scan.
//   - foreign_keys(ON) is off by default in SQLite. Left off, every ON DELETE
//     rule in the schema is silently decorative.
//   - synchronous(NORMAL) is the documented safe pairing with WAL: a crash
//     can lose the last transactions, never the database.
func Open(ctx context.Context, dir string) (*sql.DB, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating the data directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, FileName)
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(" + busyTimeout + ")" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}

	// SQLite serialises writers anyway. Holding a single connection removes a
	// whole class of intermittent "database is locked" failures, and at
	// household scale there is nothing to gain from more. Revisit only with a
	// measurement that says so.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}

	// The pragmas above are requested through the connection string. Confirm
	// they actually took: a silently ignored pragma is the kind of thing that
	// is discovered much later, by losing data.
	if err := verifyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}

	return db, nil
}

func verifyPragmas(ctx context.Context, db *sql.DB) error {
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("reading journal_mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("journal_mode is %q, want \"wal\"", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("reading foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign_keys is off, every ON DELETE rule would be decorative")
	}

	return nil
}

// Ready reports whether the database can serve queries. It is the readiness
// probe the HTTP layer exposes.
func Ready(ctx context.Context, db *sql.DB) error {
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database is not answering: %w", err)
	}
	return nil
}
