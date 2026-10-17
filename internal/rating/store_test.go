package rating_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/scope"
	"github.com/aydam2728/bookplate/internal/rating"
)

func newStore(t *testing.T) (*rating.Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return rating.NewStore(db), db
}

// household builds a library with one work and returns a member of it.
func household(t *testing.T, db *sql.DB, name string) (scope.Caller, string) {
	t.Helper()
	const now = "2026-01-01T00:00:00Z"
	lib, user, work := name+"-lib", name+"-user", name+"-work"
	stmts := [][]any{
		{`INSERT INTO library (id, name, created_at) VALUES (?, ?, ?)`, lib, name, now},
		{`INSERT INTO app_user (id, email, display_name, created_at) VALUES (?, ?, ?, ?)`,
			user, name + "@example.org", name, now},
		{`INSERT INTO membership (user_id, library_id, role, created_at)
		  VALUES (?, ?, 'owner', ?)`, user, lib, now},
		{`INSERT INTO work (id, library_id, title, created_at, updated_at)
		  VALUES (?, ?, 'A book', ?, ?)`, work, lib, now, now},
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(t.Context(), q[0].(string), q[1:]...); err != nil {
			t.Fatalf("building %s: %v", name, err)
		}
	}
	return scope.Caller{UserID: user, LibraryID: lib, Role: scope.RoleOwner}, work
}

// member adds a second person to an existing library.
func member(t *testing.T, db *sql.DB, c scope.Caller, id string, role scope.Role) scope.Caller {
	t.Helper()
	const now = "2026-01-01T00:00:00Z"
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO app_user (id, email, display_name, created_at) VALUES (?, ?, ?, ?)`,
		id, id+"@example.org", id, now); err != nil {
		t.Fatalf("creating %s: %v", id, err)
	}
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO membership (user_id, library_id, role, created_at) VALUES (?, ?, ?, ?)`,
		id, c.LibraryID, string(role), now); err != nil {
		t.Fatalf("adding %s: %v", id, err)
	}
	return scope.Caller{UserID: id, LibraryID: c.LibraryID, Role: role}
}

func TestSet_ThenMine_RoundTrips(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	in := rating.Entry{Rating: 9, Review: "Lu d'une traite.", Status: "read", Visibility: "household"}
	if err := s.Set(t.Context(), c, work, in); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Mine(t.Context(), c, work)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if got.Rating != 9 || got.Review != "Lu d'une traite." ||
		got.Status != "read" || got.Visibility != "household" {
		t.Errorf("got %+v, want the entry just saved", got)
	}
}

func TestMine_WhenNothingWasSaid_IsEmptyAndPrivate(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	got, err := s.Mine(t.Context(), c, work)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if got.Rating != 0 {
		t.Errorf("Rating = %d, want 0", got.Rating)
	}
	if got.Visibility != "private" {
		t.Errorf("Visibility = %q: an unsaved opinion defaults to private", got.Visibility)
	}
}

func TestSet_AReaderMayRate(t *testing.T) {
	// A reader cannot change what the household owns, but an opinion is not
	// a change to the catalogue.
	s, db := newStore(t)
	owner, work := household(t, db, "alpha")
	reader := member(t, db, owner, "reader", scope.RoleReader)

	if err := s.Set(t.Context(), reader, work, rating.Entry{Rating: 6}); err != nil {
		t.Errorf("a reader was refused their own opinion: %v", err)
	}
}

func TestSet_TwoPeople_AverageTheirRatings(t *testing.T) {
	s, db := newStore(t)
	owner, work := household(t, db, "alpha")
	other := member(t, db, owner, "other", scope.RoleEditor)

	if err := s.Set(t.Context(), owner, work, rating.Entry{Rating: 10}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(t.Context(), other, work, rating.Entry{Rating: 6}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.OfHousehold(t.Context(), owner, work)
	if err != nil {
		t.Fatalf("OfHousehold: %v", err)
	}
	if got.Count != 2 || got.Average != 8 {
		t.Errorf("got %+v, want 2 opinions averaging 8", got)
	}
}

func TestSet_ChangingARating_KeepsTheAverageRight(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	for _, r := range []int{10, 4, 8} {
		if err := s.Set(t.Context(), c, work, rating.Entry{Rating: r}); err != nil {
			t.Fatalf("Set %d: %v", r, err)
		}
	}

	got, err := s.OfHousehold(t.Context(), c, work)
	if err != nil {
		t.Fatalf("OfHousehold: %v", err)
	}
	if got.Count != 1 || got.Average != 8 {
		t.Errorf("got %+v, want one opinion of 8: changing a rating must "+
			"replace it, not add another", got)
	}
}

func TestSet_ClearingARating_RemovesItFromTheAverage(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	if err := s.Set(t.Context(), c, work, rating.Entry{Rating: 7}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(t.Context(), c, work, rating.Entry{Rating: 0, Status: "reading"}); err != nil {
		t.Fatalf("clearing: %v", err)
	}

	got, err := s.OfHousehold(t.Context(), c, work)
	if err != nil {
		t.Fatalf("OfHousehold: %v", err)
	}
	if got.Count != 0 {
		t.Errorf("got %+v, want nobody counted after the rating was cleared", got)
	}
}

func TestSet_RefusesAnImpossibleRating(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	for _, r := range []int{-1, 11, 99} {
		if err := s.Set(t.Context(), c, work, rating.Entry{Rating: r}); !errors.Is(err, rating.ErrInvalid) {
			t.Errorf("rating %d was accepted, err = %v", r, err)
		}
	}
}

func TestSet_RefusesAnUnknownStatus(t *testing.T) {
	s, db := newStore(t)
	c, work := household(t, db, "alpha")

	err := s.Set(t.Context(), c, work, rating.Entry{Status: "skimmed"})
	if !errors.Is(err, rating.ErrInvalid) {
		t.Errorf("an invented status was accepted, err = %v", err)
	}
}

func TestSet_CannotRateAnotherHouseholdsBook(t *testing.T) {
	s, db := newStore(t)
	_, alphaWork := household(t, db, "alpha")
	beta, _ := household(t, db, "beta")

	if err := s.Set(t.Context(), beta, alphaWork, rating.Entry{Rating: 2}); !errors.Is(err, rating.ErrNotFound) {
		t.Errorf("a member of another household rated someone else's book, err = %v", err)
	}
}

func TestMine_IsPerPerson(t *testing.T) {
	s, db := newStore(t)
	owner, work := household(t, db, "alpha")
	other := member(t, db, owner, "other", scope.RoleReader)

	if err := s.Set(t.Context(), owner, work, rating.Entry{Rating: 10, Review: "Excellent"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Mine(t.Context(), other, work)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if got.Rating != 0 || got.Review != "" {
		t.Errorf("got %+v: one person's opinion leaked into another's", got)
	}
}
