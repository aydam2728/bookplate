package covers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aydam2728/bookplate/internal/platform/id"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// Dir is the subdirectory of the data directory holding cover files, so that
// backing up one directory still captures everything.
const Dir = "covers"

// ErrNotFound reports a cover that is not in this library.
var ErrNotFound = errors.New("cover not found")

// Store keeps cover images on disk and their metadata in the database.
type Store struct {
	db      *sql.DB
	dataDir string
}

// NewStore returns a Store writing under dataDir.
func NewStore(db *sql.DB, dataDir string) *Store {
	return &Store{db: db, dataDir: dataDir}
}

// Save normalises raw and records it, returning the asset identifier.
//
// The same image saved twice in one library reuses the first row: covers of
// the same edition are identical, and a household that scans a series would
// otherwise store the publisher's logo forty times.
func (s *Store) Save(ctx context.Context, caller scope.Caller, raw []byte, sourceURL, attribution string) (string, error) {
	if err := caller.RequireEdit(); err != nil {
		return "", err
	}

	normalised, err := Normalise(raw)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(normalised.Data)
	digest := hex.EncodeToString(sum[:])

	var existing string
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM cover_asset WHERE library_id = ? AND sha256 = ?`,
		caller.LibraryID, digest).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("looking for an existing cover: %w", err)
	}

	if err := s.write(digest, normalised.Data); err != nil {
		return "", err
	}

	assetID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO cover_asset
		   (id, library_id, sha256, mime_type, width, height, byte_size,
		    source_url, attribution, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, nullif(?, ''), nullif(?, ''), ?)`,
		assetID, caller.LibraryID, digest, normalised.Mime,
		normalised.Width, normalised.Height, len(normalised.Data),
		sourceURL, attribution, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("recording the cover: %w", err)
	}
	return assetID, nil
}

// Read returns the bytes and media type of a stored cover.
func (s *Store) Read(ctx context.Context, caller scope.Caller, assetID string) ([]byte, string, error) {
	if !caller.Valid() {
		return nil, "", fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}

	var digest, mime string
	err := s.db.QueryRowContext(ctx,
		`SELECT sha256, mime_type FROM cover_asset WHERE id = ? AND library_id = ?`,
		assetID, caller.LibraryID).Scan(&digest, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("reading the cover record: %w", err)
	}

	data, err := os.ReadFile(s.pathOf(digest))
	if err != nil {
		return nil, "", fmt.Errorf("reading the cover file: %w", err)
	}
	return data, mime, nil
}

// Attach points an edition at a cover, both scoped to the caller's library.
func (s *Store) Attach(ctx context.Context, caller scope.Caller, editionID, assetID string) error {
	if err := caller.RequireEdit(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE edition SET cover_asset_id = ?, updated_at = ?
		  WHERE id = ? AND library_id = ?
		    AND EXISTS (SELECT 1 FROM cover_asset
		                 WHERE id = ? AND library_id = ?)`,
		assetID, time.Now().UTC().Format(time.RFC3339),
		editionID, caller.LibraryID, assetID, caller.LibraryID)
	if err != nil {
		return fmt.Errorf("attaching the cover: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("attaching the cover: %w", err)
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

// pathOf shards by the first two characters of the digest: one directory
// holding twenty thousand files is slow to list on an SD card.
func (s *Store) pathOf(digest string) string {
	return filepath.Join(s.dataDir, Dir, digest[:2], digest+".jpg")
}

func (s *Store) write(digest string, data []byte) error {
	path := s.pathOf(digest)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating the cover directory: %w", err)
	}
	// Write then rename, so a crash never leaves a half-written cover that
	// matches a digest it does not have.
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o640); err != nil {
		return fmt.Errorf("writing the cover: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("writing the cover: %w", err)
	}
	return nil
}
