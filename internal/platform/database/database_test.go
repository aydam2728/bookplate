package database_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/database"
)

func TestOpen_CreatesTheDatabaseFile(t *testing.T) {
	dir := t.TempDir()

	db, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := os.Stat(filepath.Join(dir, database.FileName)); err != nil {
		t.Errorf("the database file was not created: %v", err)
	}
}

func TestOpen_CreatesTheDataDirectory(t *testing.T) {
	// An operator mounting a fresh volume gets an empty path, not a directory.
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	db, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open refused to create a missing data directory: %v", err)
	}
	defer func() { _ = db.Close() }()
}

func TestOpen_EnablesWriteAheadLogging(t *testing.T) {
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var mode string
	if err := db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("reading journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want \"wal\": readers would block during a bulk scan", mode)
	}
}

func TestOpen_EnforcesForeignKeys(t *testing.T) {
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	for _, stmt := range []string{
		`CREATE TABLE parent_row (id TEXT PRIMARY KEY) STRICT`,
		`CREATE TABLE child_row (
			id        TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL REFERENCES parent_row(id) ON DELETE CASCADE
		) STRICT`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("preparing the fixture: %v", err)
		}
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO child_row (id, parent_id) VALUES ('c1', 'no-such-parent')`)
	if err == nil {
		t.Fatal("a row referencing a missing parent was accepted: foreign keys are " +
			"off, so every ON DELETE rule in the schema is decorative")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Errorf("the insert failed for an unrelated reason: %v", err)
	}
}

func TestOpen_SetsABusyTimeout(t *testing.T) {
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var timeout int
	if err := db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("reading busy_timeout: %v", err)
	}
	if timeout == 0 {
		t.Error("busy_timeout is 0: a routine lock collision becomes a visible error")
	}
}

func TestReady_WhenTheDatabaseIsOpen_ReturnsNil(t *testing.T) {
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := database.Ready(t.Context(), db); err != nil {
		t.Errorf("Ready reported a problem on a healthy database: %v", err)
	}
}

func TestReady_WhenTheDatabaseIsClosed_ReportsIt(t *testing.T) {
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.Close()

	if err := database.Ready(t.Context(), db); err == nil {
		t.Error("Ready reported a closed database as usable")
	}
}
