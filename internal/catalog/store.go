// Package catalog holds the books: works, editions, and the copies on a
// shelf. See docs/adr/0004-work-edition-copy.md.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aydam2728/bookplate/internal/platform/id"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// ErrInvalid reports input a person can fix.
var ErrInvalid = errors.New("invalid")

// Book is one copy, flattened for display: a shelf view wants a title and an
// author, not three joined rows.
type Book struct {
	CopyID    string `json:"copyId"`
	WorkID    string `json:"workId"`
	EditionID string `json:"editionId"`
	Title     string `json:"title"`
	Authors   string `json:"authors"`
	Publisher string `json:"publisher"`
	Published string `json:"published"`
	ISBN13    string `json:"isbn13"`
	CoverID   string `json:"coverId"`
	PlaceID   string `json:"placeId"`
	PlaceName string `json:"placeName"`
	Summary   string `json:"summary"`
	Notes     string `json:"notes"`
	AddedAt   string `json:"addedAt"`
}

// AddInput is what a person types or scans.
type AddInput struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	ISBN      string `json:"isbn"`
	Publisher string `json:"publisher"`
	Published string `json:"published"`
	CoverURL  string `json:"coverUrl"`
	PlaceID   string `json:"placeId"`
}

// Store reads and writes the catalogue.
type Store struct{ db *sql.DB }

// NewStore returns a Store over db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Add records one copy, creating the work and the edition if they are new.
//
// Every statement is scoped by caller.LibraryID. The caller is a parameter
// rather than something fished out of a context, so a query without a scope
// does not compile.
func (s *Store) Add(ctx context.Context, caller scope.Caller, in AddInput) (Book, error) {
	if err := caller.RequireEdit(); err != nil {
		return Book{}, err
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Book{}, fmt.Errorf("%w: a title is required", ErrInvalid)
	}

	// A book with no ISBN must catalogue as easily as a recent reprint. That
	// is a structural requirement, not an edge case.
	var isbn13 string
	if strings.TrimSpace(in.ISBN) != "" {
		normalised, err := NormaliseISBN(in.ISBN)
		if err != nil {
			return Book{}, err
		}
		isbn13 = normalised
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("adding a book: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	editionID, err := findEdition(ctx, tx, caller, isbn13)
	if err != nil {
		return Book{}, err
	}
	if editionID == "" {
		editionID, err = createWorkAndEdition(ctx, tx, caller, in, title, isbn13, now)
		if err != nil {
			return Book{}, err
		}
	}

	copyID, err := id.New()
	if err != nil {
		return Book{}, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO copy (id, library_id, edition_id, place_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, nullif(?, ''), 'owned', ?, ?)`,
		copyID, caller.LibraryID, editionID, strings.TrimSpace(in.PlaceID), now, now)
	if err != nil {
		return Book{}, fmt.Errorf("adding a copy: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("adding a book: %w", err)
	}
	return s.ByCopyID(ctx, caller, copyID)
}

// findEdition looks for an edition already recorded under this ISBN in this
// library. Finding one means the household owns a second copy of the same
// edition, not a duplicate to refuse.
func findEdition(ctx context.Context, tx *sql.Tx, caller scope.Caller, isbn13 string) (string, error) {
	if isbn13 == "" {
		return "", nil
	}
	var editionID string
	err := tx.QueryRowContext(ctx,
		`SELECT edition_id FROM edition_identifier
		  WHERE library_id = ? AND scheme = 'isbn13' AND value = ?`,
		caller.LibraryID, isbn13).Scan(&editionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("looking for an existing edition: %w", err)
	}
	return editionID, nil
}

func createWorkAndEdition(
	ctx context.Context, tx *sql.Tx, caller scope.Caller,
	in AddInput, title, isbn13, now string,
) (string, error) {
	workID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO work (id, library_id, title, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		workID, caller.LibraryID, title, now, now)
	if err != nil {
		return "", fmt.Errorf("creating the work: %w", err)
	}

	if author := strings.TrimSpace(in.Author); author != "" {
		authorID, err := upsertAuthor(ctx, tx, caller, author, now)
		if err != nil {
			return "", err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO work_author (work_id, author_id, library_id, role, position)
			 VALUES (?, ?, ?, 'author', 0)`,
			workID, authorID, caller.LibraryID)
		if err != nil {
			return "", fmt.Errorf("linking the author: %w", err)
		}
	}

	editionID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO edition (id, library_id, work_id, publisher, published_date, created_at, updated_at)
		 VALUES (?, ?, ?, nullif(?, ''), nullif(?, ''), ?, ?)`,
		editionID, caller.LibraryID, workID,
		strings.TrimSpace(in.Publisher), strings.TrimSpace(in.Published), now, now)
	if err != nil {
		return "", fmt.Errorf("creating the edition: %w", err)
	}

	if isbn13 != "" {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO edition_identifier (edition_id, library_id, scheme, value)
			 VALUES (?, ?, 'isbn13', ?)`,
			editionID, caller.LibraryID, isbn13)
		if err != nil {
			return "", fmt.Errorf("recording the ISBN: %w", err)
		}
	}
	return editionID, nil
}

func upsertAuthor(ctx context.Context, tx *sql.Tx, caller scope.Caller, name, now string) (string, error) {
	sortName := sortableName(name)

	var authorID string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM author WHERE library_id = ? AND sort_name = ?`,
		caller.LibraryID, sortName).Scan(&authorID)
	if err == nil {
		return authorID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("looking for the author: %w", err)
	}

	authorID, err = id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO author (id, library_id, name, sort_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		authorID, caller.LibraryID, name, sortName, now)
	if err != nil {
		return "", fmt.Errorf("creating the author: %w", err)
	}
	return authorID, nil
}

// sortableName turns "J. R. R. Tolkien" into "tolkien, j. r. r." so a shelf
// sorts the way a reader expects.
func sortableName(name string) string {
	fields := strings.Fields(name)
	if len(fields) < 2 {
		return strings.ToLower(name)
	}
	last := fields[len(fields)-1]
	rest := strings.Join(fields[:len(fields)-1], " ")
	return strings.ToLower(last + ", " + rest)
}

const selectBooks = `
	SELECT c.id, w.id, e.id, w.title,
	       coalesce(group_concat(a.name, ', '), ''),
	       coalesce(e.publisher, ''), coalesce(e.published_date, ''),
	       coalesce(i.value, ''), c.created_at, coalesce(e.cover_asset_id, ''),
	       coalesce(c.place_id, ''), coalesce(pl.name, ''),
	       coalesce(w.summary, ''), coalesce(c.notes, '')
	  FROM copy c
	  JOIN edition e ON e.id = c.edition_id AND e.library_id = c.library_id
	  JOIN work    w ON w.id = e.work_id    AND w.library_id = c.library_id
	  LEFT JOIN work_author wa ON wa.work_id = w.id AND wa.library_id = c.library_id
	  LEFT JOIN author      a  ON a.id = wa.author_id AND a.library_id = c.library_id
	  LEFT JOIN place pl ON pl.id = c.place_id AND pl.library_id = c.library_id
	  LEFT JOIN edition_identifier i
	         ON i.edition_id = e.id AND i.library_id = c.library_id AND i.scheme = 'isbn13'
	 WHERE c.library_id = ? AND c.deleted_at IS NULL`

// List returns the copies in the caller's library, most recently added first.
func (s *Store) List(ctx context.Context, caller scope.Caller) ([]Book, error) {
	if !caller.Valid() {
		return nil, fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}

	rows, err := s.db.QueryContext(ctx,
		selectBooks+` GROUP BY c.id ORDER BY c.created_at DESC, c.id DESC`,
		caller.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("listing books: %w", err)
	}
	defer func() { _ = rows.Close() }()

	books := []Book{}
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.CopyID, &b.WorkID, &b.EditionID, &b.Title, &b.Authors,
			&b.Publisher, &b.Published, &b.ISBN13, &b.AddedAt, &b.CoverID,
			&b.PlaceID, &b.PlaceName, &b.Summary, &b.Notes); err != nil {
			return nil, fmt.Errorf("listing books: %w", err)
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing books: %w", err)
	}
	return books, nil
}

// ByCopyID returns one copy of the caller's library.
func (s *Store) ByCopyID(ctx context.Context, caller scope.Caller, copyID string) (Book, error) {
	var b Book
	err := s.db.QueryRowContext(ctx,
		selectBooks+` AND c.id = ? GROUP BY c.id`,
		caller.LibraryID, copyID).Scan(&b.CopyID, &b.WorkID, &b.EditionID, &b.Title, &b.Authors,
		&b.Publisher, &b.Published, &b.ISBN13, &b.AddedAt, &b.CoverID,
		&b.PlaceID, &b.PlaceName, &b.Summary, &b.Notes)
	if err != nil {
		return Book{}, fmt.Errorf("reading back the copy: %w", err)
	}
	return b, nil
}

// Edit is what a person may change about a copy from its detail view.
// A nil field is left alone, so changing the place does not wipe the notes.
type Edit struct {
	Summary *string `json:"summary"`
	Notes   *string `json:"notes"`
	PlaceID *string `json:"placeId"`
}

// Update changes a copy and the work it belongs to.
//
// The summary lives on the work, the notes and the place on the copy: two
// people owning the same book share the description and keep their own notes.
func (s *Store) Update(ctx context.Context, caller scope.Caller, copyID string, in Edit) (Book, error) {
	if err := caller.RequireEdit(); err != nil {
		return Book{}, err
	}

	current, err := s.ByCopyID(ctx, caller, copyID)
	if err != nil {
		return Book{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("updating the book: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	if in.Summary != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE work SET summary = nullif(?, ''), updated_at = ?
			  WHERE id = ? AND library_id = ?`,
			strings.TrimSpace(*in.Summary), now, current.WorkID, caller.LibraryID)
		if err != nil {
			return Book{}, fmt.Errorf("updating the summary: %w", err)
		}
	}
	if in.Notes != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE copy SET notes = nullif(?, ''), updated_at = ?
			  WHERE id = ? AND library_id = ?`,
			strings.TrimSpace(*in.Notes), now, copyID, caller.LibraryID)
		if err != nil {
			return Book{}, fmt.Errorf("updating the notes: %w", err)
		}
	}
	if in.PlaceID != nil {
		// An empty identifier un-files the copy rather than failing: "not
		// filed yet" is a state a person chooses.
		place := strings.TrimSpace(*in.PlaceID)
		if place != "" {
			var exists int
			err = tx.QueryRowContext(ctx,
				`SELECT count(*) FROM place
				  WHERE id = ? AND library_id = ? AND deleted_at IS NULL`,
				place, caller.LibraryID).Scan(&exists)
			if err != nil {
				return Book{}, fmt.Errorf("looking for the place: %w", err)
			}
			if exists == 0 {
				return Book{}, fmt.Errorf("%w: no such place", ErrInvalid)
			}
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE copy SET place_id = nullif(?, ''), updated_at = ?
			  WHERE id = ? AND library_id = ?`,
			place, now, copyID, caller.LibraryID)
		if err != nil {
			return Book{}, fmt.Errorf("moving the copy: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("updating the book: %w", err)
	}
	return s.ByCopyID(ctx, caller, copyID)
}
