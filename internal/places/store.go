// Package places holds the tree of physical locations: where a copy actually
// sits. No level is imposed by the software, because a household does not
// organise itself the way a cataloguer would.
package places

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

// ErrNotFound reports a place that is not in the caller's library.
var ErrNotFound = errors.New("place not found")

// ErrWouldCycle reports a move that would make a place its own ancestor.
var ErrWouldCycle = errors.New("a place cannot be moved inside itself")

// Place is one node of the tree.
type Place struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Depth    int    `json:"depth"`
	// Copies counts the copies in this place and everything under it.
	Copies int `json:"copies"`
}

// Store reads and writes the tree.
type Store struct{ db *sql.DB }

// NewStore returns a Store over db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Create adds a place under parentID, or at the root when parentID is empty.
func (s *Store) Create(ctx context.Context, caller scope.Caller, parentID, name, kind string) (Place, error) {
	if err := caller.RequireEdit(); err != nil {
		return Place{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Place{}, fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	if kind == "" {
		kind = "other"
	}

	parentPath := "/"
	depth := 0
	if parentID != "" {
		parent, err := s.byID(ctx, caller, parentID)
		if err != nil {
			return Place{}, err
		}
		parentPath = parent.Path
		depth = parent.Depth + 1
	}

	placeID, err := id.New()
	if err != nil {
		return Place{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO place (id, library_id, parent_id, name, kind, path, depth, created_at)
		 VALUES (?, ?, nullif(?, ''), ?, ?, ?, ?, ?)`,
		placeID, caller.LibraryID, parentID, name, kind,
		parentPath+placeID+"/", depth, now)
	if err != nil {
		return Place{}, fmt.Errorf("creating the place: %w", err)
	}
	return s.byID(ctx, caller, placeID)
}

// List returns the whole tree in reading order: a parent always comes before
// its children, because sorting by path does that for free.
func (s *Store) List(ctx context.Context, caller scope.Caller) ([]Place, error) {
	if !caller.Valid() {
		return nil, fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, coalesce(parent_id, ''), name, coalesce(kind, 'other'),
		        path, depth, subtree_copy_count
		   FROM place
		  WHERE library_id = ? AND deleted_at IS NULL
		  ORDER BY path`, caller.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("listing places: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Place{}
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Name, &p.Kind,
			&p.Path, &p.Depth, &p.Copies); err != nil {
			return nil, fmt.Errorf("listing places: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing places: %w", err)
	}
	return out, nil
}

// Rename changes a name and nothing else.
func (s *Store) Rename(ctx context.Context, caller scope.Caller, placeID, name string) error {
	if err := caller.RequireEdit(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE place SET name = ? WHERE id = ? AND library_id = ? AND deleted_at IS NULL`,
		name, placeID, caller.LibraryID)
	if err != nil {
		return fmt.Errorf("renaming the place: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) byID(ctx context.Context, caller scope.Caller, placeID string) (Place, error) {
	var p Place
	err := s.db.QueryRowContext(ctx,
		`SELECT id, coalesce(parent_id, ''), name, coalesce(kind, 'other'),
		        path, depth, subtree_copy_count
		   FROM place
		  WHERE id = ? AND library_id = ? AND deleted_at IS NULL`,
		placeID, caller.LibraryID).Scan(&p.ID, &p.ParentID, &p.Name, &p.Kind,
		&p.Path, &p.Depth, &p.Copies)
	if errors.Is(err, sql.ErrNoRows) {
		return Place{}, ErrNotFound
	}
	if err != nil {
		return Place{}, fmt.Errorf("reading the place: %w", err)
	}
	return p, nil
}

// Move re-parents a place and everything under it.
//
// Deliberately not a trigger: it rewrites the path of a whole subtree, and
// ADR 0008 keeps that in application code where it can be read.
func (s *Store) Move(ctx context.Context, caller scope.Caller, placeID, newParentID string) error {
	if err := caller.RequireEdit(); err != nil {
		return err
	}
	if placeID == newParentID {
		return ErrWouldCycle
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("moving the place: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	moving, err := readPlace(ctx, tx, caller, placeID)
	if err != nil {
		return err
	}

	newPrefix := "/"
	newDepth := 0
	if newParentID != "" {
		parent, err := readPlace(ctx, tx, caller, newParentID)
		if err != nil {
			return err
		}
		// The new parent must not already be inside what is being moved,
		// or the subtree would become its own ancestor and disappear.
		if strings.HasPrefix(parent.Path, moving.Path) {
			return ErrWouldCycle
		}
		newPrefix = parent.Path
		newDepth = parent.Depth + 1
	}

	oldPrefix := moving.Path
	target := newPrefix + placeID + "/"
	delta := newDepth - moving.Depth

	_, err = tx.ExecContext(ctx,
		`UPDATE place
		    SET path  = ? || substr(path, length(?) + 1),
		        depth = depth + ?
		  WHERE library_id = ? AND substr(path, 1, length(?)) = ?`,
		target, oldPrefix, delta, caller.LibraryID, oldPrefix, oldPrefix)
	if err != nil {
		return fmt.Errorf("rewriting the subtree: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE place SET parent_id = nullif(?, '') WHERE id = ? AND library_id = ?`,
		newParentID, placeID, caller.LibraryID)
	if err != nil {
		return fmt.Errorf("re-parenting the place: %w", err)
	}

	if err := recount(ctx, tx, caller); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("moving the place: %w", err)
	}
	return nil
}

// AssignCopies files copies into a place, or un-files them when placeID is
// empty. Selecting two hundred copies and re-shelving them in one action is
// the point: a household reorganises a wall, not a book.
func (s *Store) AssignCopies(ctx context.Context, caller scope.Caller, copyIDs []string, placeID string) (int, error) {
	if err := caller.RequireEdit(); err != nil {
		return 0, err
	}
	if len(copyIDs) == 0 {
		return 0, nil
	}
	if placeID != "" {
		if _, err := s.byID(ctx, caller, placeID); err != nil {
			return 0, err
		}
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(copyIDs)), ",")
	args := []any{sql.NullString{String: placeID, Valid: placeID != ""},
		time.Now().UTC().Format(time.RFC3339), caller.LibraryID}
	for _, c := range copyIDs {
		args = append(args, c)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE copy SET place_id = ?, updated_at = ?
		  WHERE library_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("filing the copies: %w", err)
	}
	moved, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("filing the copies: %w", err)
	}
	return int(moved), nil
}

// Delete removes a leaf. Copies filed there are un-filed rather than lost:
// losing a shelf must never lose a book.
func (s *Store) Delete(ctx context.Context, caller scope.Caller, placeID string) error {
	if err := caller.RequireEdit(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting the place: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := readPlace(ctx, tx, caller, placeID); err != nil {
		return err
	}

	var children int
	err = tx.QueryRowContext(ctx,
		`SELECT count(*) FROM place
		  WHERE library_id = ? AND parent_id = ? AND deleted_at IS NULL`,
		caller.LibraryID, placeID).Scan(&children)
	if err != nil {
		return fmt.Errorf("counting the children: %w", err)
	}
	if children > 0 {
		return fmt.Errorf("%w: empty it first, %d places are inside", ErrInvalid, children)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE copy SET place_id = NULL, updated_at = ?
		  WHERE library_id = ? AND place_id = ?`,
		time.Now().UTC().Format(time.RFC3339), caller.LibraryID, placeID); err != nil {
		return fmt.Errorf("un-filing the copies: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE place SET deleted_at = ? WHERE id = ? AND library_id = ?`,
		time.Now().UTC().Format(time.RFC3339), placeID, caller.LibraryID); err != nil {
		return fmt.Errorf("deleting the place: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("deleting the place: %w", err)
	}
	return nil
}

// recount rebuilds every counter in the library from the copies themselves.
//
// A move invalidates the counters of two subtrees at once, and the arithmetic
// to patch them incrementally is the kind nobody can check by reading. At a
// few hundred places this is one query and obviously right.
func recount(ctx context.Context, tx *sql.Tx, caller scope.Caller) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE place
		    SET subtree_copy_count = (
		        SELECT count(*)
		          FROM copy c
		          JOIN place d ON d.id = c.place_id AND d.library_id = c.library_id
		         WHERE c.library_id = place.library_id
		           AND c.deleted_at IS NULL
		           AND substr(d.path, 1, length(place.path)) = place.path)
		  WHERE library_id = ?`, caller.LibraryID)
	if err != nil {
		return fmt.Errorf("recounting the places: %w", err)
	}
	return nil
}

func readPlace(ctx context.Context, tx *sql.Tx, caller scope.Caller, placeID string) (Place, error) {
	var p Place
	err := tx.QueryRowContext(ctx,
		`SELECT id, coalesce(parent_id, ''), name, coalesce(kind, 'other'),
		        path, depth, subtree_copy_count
		   FROM place
		  WHERE id = ? AND library_id = ? AND deleted_at IS NULL`,
		placeID, caller.LibraryID).Scan(&p.ID, &p.ParentID, &p.Name, &p.Kind,
		&p.Path, &p.Depth, &p.Copies)
	if errors.Is(err, sql.ErrNoRows) {
		return Place{}, ErrNotFound
	}
	if err != nil {
		return Place{}, fmt.Errorf("reading the place: %w", err)
	}
	return p, nil
}
