// Package rating holds the personal layer: what each person thinks of a
// work, as opposed to what the household owns.
package rating

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// ErrInvalid reports input a person can fix.
var ErrInvalid = errors.New("invalid")

// ErrNotFound reports a work that is not in the caller's library.
var ErrNotFound = errors.New("work not found")

// Max is the highest rating, in half stars: 10 is five stars.
const Max = 10

// Statuses a reader can be in. An empty status means they have not said.
var Statuses = []string{"to_read", "reading", "read", "abandoned"}

// Entry is one person's opinion of one work.
type Entry struct {
	// Rating is in half stars, 1 to 10, or 0 when unrated.
	Rating     int    `json:"rating"`
	Review     string `json:"review"`
	Visibility string `json:"visibility"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updatedAt"`
}

// Household is what everyone in the library thinks, in aggregate.
type Household struct {
	// Average is in half stars, 0 when nobody has rated it.
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// Store reads and writes the personal layer.
type Store struct{ db *sql.DB }

// NewStore returns a Store over db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Set records what the caller thinks of a work.
//
// A reader may rate and review; only an editor may change the catalogue. That
// asymmetry is deliberate: an opinion is not a change to what the household
// owns.
func (s *Store) Set(ctx context.Context, caller scope.Caller, workID string, in Entry) error {
	if !caller.Valid() {
		return fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}
	if in.Rating < 0 || in.Rating > Max {
		return fmt.Errorf("%w: a rating runs from 1 to %d half stars, or 0 for none",
			ErrInvalid, Max)
	}
	if in.Status != "" && !validStatus(in.Status) {
		return fmt.Errorf("%w: %q is not a reading status", ErrInvalid, in.Status)
	}
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Visibility != "private" && in.Visibility != "household" {
		return fmt.Errorf("%w: visibility is private or household", ErrInvalid)
	}

	// The work has to be in the caller's library, or one household could
	// rate another's books by guessing an identifier.
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM work WHERE id = ? AND library_id = ?`,
		workID, caller.LibraryID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("looking for the work: %w", err)
	}
	if exists == 0 {
		return ErrNotFound
	}

	var rating any
	if in.Rating > 0 {
		rating = in.Rating
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_work
		   (user_id, work_id, library_id, rating, review, visibility, status, updated_at)
		 VALUES (?, ?, ?, ?, nullif(?, ''), ?, nullif(?, ''), ?)
		 ON CONFLICT (user_id, work_id) DO UPDATE SET
		   rating     = excluded.rating,
		   review     = excluded.review,
		   visibility = excluded.visibility,
		   status     = excluded.status,
		   updated_at = excluded.updated_at`,
		caller.UserID, workID, caller.LibraryID, rating,
		strings.TrimSpace(in.Review), in.Visibility, in.Status, now)
	if err != nil {
		return fmt.Errorf("saving the rating: %w", err)
	}
	return nil
}

// Mine returns what the caller thinks of a work, zero when they have not
// said.
func (s *Store) Mine(ctx context.Context, caller scope.Caller, workID string) (Entry, error) {
	if !caller.Valid() {
		return Entry{}, fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}

	var e Entry
	var rating sql.NullInt64
	var review, status sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT coalesce(rating, 0), review, visibility, status, updated_at
		   FROM user_work
		  WHERE user_id = ? AND work_id = ? AND library_id = ?`,
		caller.UserID, workID, caller.LibraryID).
		Scan(&rating, &review, &e.Visibility, &status, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{Visibility: "private"}, nil
	}
	if err != nil {
		return Entry{}, fmt.Errorf("reading the rating: %w", err)
	}
	e.Rating = int(rating.Int64)
	e.Review = review.String
	e.Status = status.String
	return e, nil
}

// OfHousehold returns the average everyone in the library has given.
//
// It reads the counters on the work rather than scanning the opinions,
// because a shelf shows this on every row.
func (s *Store) OfHousehold(ctx context.Context, caller scope.Caller, workID string) (Household, error) {
	if !caller.Valid() {
		return Household{}, fmt.Errorf("%w: incomplete caller", scope.ErrForbidden)
	}

	var count, sum int
	err := s.db.QueryRowContext(ctx,
		`SELECT rating_count, rating_sum FROM work WHERE id = ? AND library_id = ?`,
		workID, caller.LibraryID).Scan(&count, &sum)
	if errors.Is(err, sql.ErrNoRows) {
		return Household{}, ErrNotFound
	}
	if err != nil {
		return Household{}, fmt.Errorf("reading the household rating: %w", err)
	}
	if count == 0 {
		return Household{}, nil
	}
	return Household{Average: float64(sum) / float64(count), Count: count}, nil
}

func validStatus(s string) bool {
	for _, known := range Statuses {
		if known == s {
			return true
		}
	}
	return false
}
