package database_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aydam2728/bookplate/internal/platform/database"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fixtures(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func TestMigrate_AppliesInFilenameOrder(t *testing.T) {
	db := openDB(t)
	fsys := fixtures(map[string]string{
		"0002_second.sql": `ALTER TABLE a ADD COLUMN extra TEXT`,
		"0001_first.sql":  `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
	})

	applied, err := database.Migrate(t.Context(), db, fsys)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	want := []string{"0001_first.sql", "0002_second.sql"}
	if len(applied) != len(want) || applied[0] != want[0] || applied[1] != want[1] {
		t.Fatalf("applied %v, want %v: 0002 alters a table 0001 creates", applied, want)
	}
}

func TestMigrate_RunTwice_AppliesNothingTheSecondTime(t *testing.T) {
	db := openDB(t)
	fsys := fixtures(map[string]string{
		"0001_first.sql": `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
	})

	if _, err := database.Migrate(t.Context(), db, fsys); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	applied, err := database.Migrate(t.Context(), db, fsys)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("the second run applied %v, want nothing: startup must be idempotent", applied)
	}
}

func TestMigrate_WhenAnAppliedMigrationChanged_Refuses(t *testing.T) {
	db := openDB(t)
	original := fixtures(map[string]string{
		"0001_first.sql": `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
	})
	if _, err := database.Migrate(t.Context(), db, original); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	edited := fixtures(map[string]string{
		"0001_first.sql": `CREATE TABLE a (id TEXT PRIMARY KEY, sneaky TEXT) STRICT`,
	})

	_, err := database.Migrate(t.Context(), db, edited)
	if !errors.Is(err, database.ErrMigrationChanged) {
		t.Fatalf("Migrate accepted an edited migration, err = %v: every existing "+
			"instance would be left on a schema nobody can reproduce", err)
	}
	if !strings.Contains(err.Error(), "0001_first.sql") {
		t.Errorf("the error does not name the migration: %v", err)
	}
}

func TestMigrate_WhenTheDatabaseIsNewer_Refuses(t *testing.T) {
	db := openDB(t)
	newer := fixtures(map[string]string{
		"0001_first.sql":  `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
		"0002_second.sql": `CREATE TABLE b (id TEXT PRIMARY KEY) STRICT`,
	})
	if _, err := database.Migrate(t.Context(), db, newer); err != nil {
		t.Fatalf("migrating with the newer set: %v", err)
	}

	// An operator rolls the image back to a version that predates 0002.
	older := fixtures(map[string]string{
		"0001_first.sql": `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
	})

	_, err := database.Migrate(t.Context(), db, older)
	if !errors.Is(err, database.ErrNewerDatabase) {
		t.Fatalf("an older binary accepted a newer database, err = %v: silently "+
			"rolling a schema back is how data is lost", err)
	}
}

func TestMigrate_WhenAMigrationFails_RecordsNothing(t *testing.T) {
	db := openDB(t)
	broken := fixtures(map[string]string{
		"0001_first.sql": `CREATE TABLE a (id TEXT PRIMARY KEY) STRICT;
		                   CREATE TABLE a (id TEXT PRIMARY KEY) STRICT`,
	})

	if _, err := database.Migrate(t.Context(), db, broken); err == nil {
		t.Fatal("Migrate reported success on a migration that cannot run")
	}

	var count int
	err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM schema_migration WHERE name = '0001_first.sql'`).Scan(&count)
	if err != nil {
		t.Fatalf("reading schema_migration: %v", err)
	}
	if count != 0 {
		t.Error("a migration that failed was recorded as applied, so a retry would skip it")
	}
}

func TestMigrate_TheRealSchemaApplies(t *testing.T) {
	db := openDB(t)

	applied, err := database.Migrate(t.Context(), db, database.Migrations())
	if err != nil {
		t.Fatalf("the embedded schema does not apply: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("no migration was applied: the embedded set is empty")
	}

	// The scoping rule only holds if foreign keys are actually enforced on the
	// real tables, not just on a fixture.
	_, err = db.ExecContext(t.Context(),
		`INSERT INTO membership (user_id, library_id, role, created_at)
		 VALUES ('nobody', 'nowhere', 'owner', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Error("a membership was accepted for a user and a library that do not exist")
	}
}
