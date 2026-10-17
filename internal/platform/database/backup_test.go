package database_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/database"
	_ "modernc.org/sqlite"
)

func TestBackup_WritesAUsableCopy(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	path, err := database.Backup(t.Context(), db, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// A backup nobody can open is not a backup.
	copyDB, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening the backup: %v", err)
	}
	defer func() { _ = copyDB.Close() }()

	var count int
	err = copyDB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM schema_migration`).Scan(&count)
	if err != nil {
		t.Fatalf("querying the backup: %v", err)
	}
	if count == 0 {
		t.Error("the backup has no migration history, so it did not capture the schema")
	}
}

func TestBackup_KeepsOnlyTheMostRecentCopies(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for i := 0; i < database.BackupsKept+3; i++ {
		if _, err := database.Backup(t.Context(), db, dir); err != nil {
			t.Fatalf("Backup %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(dir, database.BackupsDir))
	if err != nil {
		t.Fatalf("listing the backups: %v", err)
	}
	if len(entries) != database.BackupsKept {
		t.Errorf("kept %d copies, want %d: an unbounded backup directory fills "+
			"the SD card of the reference machine", len(entries), database.BackupsKept)
	}
}
