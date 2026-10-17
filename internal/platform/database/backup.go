package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BackupsDir is where copies are written, inside the data directory, so that
// backing up one directory still captures everything.
const BackupsDir = "backups"

// BackupsKept bounds how many copies are retained. The reference machine is a
// Raspberry Pi with an SD card; an unbounded backup directory fills it and
// takes the instance down with it.
const BackupsKept = 5

// Backup writes a consistent copy of the database into dir/backups and prunes
// older copies, returning the path it wrote.
//
// It uses SQLite's VACUUM INTO rather than copying the file: VACUUM INTO runs
// online, takes its own consistent view, and writes a compacted database. A
// plain file copy of a live database can capture a write in progress.
func Backup(ctx context.Context, db *sql.DB, dir string) (string, error) {
	target := filepath.Join(dir, BackupsDir)
	if err := os.MkdirAll(target, 0o750); err != nil {
		return "", fmt.Errorf("creating %q: %w", target, err)
	}

	now := time.Now().UTC()
	name := fmt.Sprintf("bookplate-%s-%09d.db",
		now.Format("20060102T150405"), now.Nanosecond())
	path := filepath.Join(target, name)

	// VACUUM INTO refuses to overwrite, which is the behaviour we want: a
	// backup that silently replaces another is not a backup.
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return "", fmt.Errorf("backing up to %q: %w", path, err)
	}

	if err := pruneBackups(target, BackupsKept); err != nil {
		// The copy exists; failing to tidy up is not a reason to refuse to
		// start. Report it so the caller can log it.
		return path, fmt.Errorf("backup written to %q, but pruning failed: %w", path, err)
	}
	return path, nil
}

// pruneBackups keeps the most recent keep files and removes the rest. Names
// are timestamped, so sorting them lexically sorts them chronologically.
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("listing %q: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return nil
	}

	sort.Strings(names)
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("removing %q: %w", name, err)
		}
	}
	return nil
}
