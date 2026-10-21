package catalog_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/catalog"
	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

func newStore(t *testing.T) (*catalog.Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return catalog.NewStore(db), db
}

// household creates a library and returns an owner of it.
func household(t *testing.T, db *sql.DB, name string) scope.Caller {
	t.Helper()
	libraryID := name + "-library"
	userID := name + "-user"
	const now = "2026-01-01T00:00:00Z"
	for _, q := range []struct {
		stmt string
		args []any
	}{
		{`INSERT INTO library (id, name, created_at) VALUES (?, ?, ?)`,
			[]any{libraryID, name, now}},
		{`INSERT INTO app_user (id, email, display_name, created_at)
		  VALUES (?, ?, ?, ?)`,
			[]any{userID, name + "@example.org", name, now}},
		{`INSERT INTO membership (user_id, library_id, role, created_at)
		  VALUES (?, ?, 'owner', ?)`,
			[]any{userID, libraryID, now}},
	} {
		if _, err := db.ExecContext(t.Context(), q.stmt, q.args...); err != nil {
			t.Fatalf("building the %s household: %v", name, err)
		}
	}
	return scope.Caller{UserID: userID, LibraryID: libraryID, Role: scope.RoleOwner}
}

func TestAdd_ThenList_ShowsTheBook(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	added, err := s.Add(t.Context(), owner, catalog.AddInput{
		Title: "The Hobbit", Author: "J. R. R. Tolkien",
		ISBN: "978-0-14-032872-1", Publisher: "Puffin",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.Title != "The Hobbit" || added.Authors != "J. R. R. Tolkien" {
		t.Errorf("the copy read back wrong: %+v", added)
	}
	if added.ISBN13 != "9780140328721" {
		t.Errorf("ISBN13 = %q, want the normalised form", added.ISBN13)
	}

	books, err := s.List(t.Context(), owner, catalog.Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(books) != 1 || books[0].CopyID != added.CopyID {
		t.Errorf("List returned %d books, want the one just added", len(books))
	}
}

func TestAdd_WithoutAnISBN_Works(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	// An 1890 edition has no ISBN and must catalogue as easily as a reprint.
	added, err := s.Add(t.Context(), owner, catalog.AddInput{
		Title: "Les Travailleurs de la mer", Author: "Victor Hugo",
	})
	if err != nil {
		t.Fatalf("a book with no ISBN was refused: %v", err)
	}
	if added.ISBN13 != "" {
		t.Errorf("ISBN13 = %q, want empty", added.ISBN13)
	}
}

func TestAdd_TheSameISBNTwice_IsASecondCopy(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	in := catalog.AddInput{Title: "The Hobbit", ISBN: "9780140328721"}
	first, err := s.Add(t.Context(), owner, in)
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	second, err := s.Add(t.Context(), owner, in)
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}

	if first.CopyID == second.CopyID {
		t.Fatal("the second add reused the first copy")
	}
	if first.WorkID != second.WorkID {
		t.Error("the same ISBN produced two works: owning two copies is not " +
			"owning two different books")
	}

	var editions int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM edition`).Scan(&editions); err != nil {
		t.Fatalf("counting editions: %v", err)
	}
	if editions != 1 {
		t.Errorf("%d editions, want 1", editions)
	}
}

func TestAdd_WhenTheTitleIsBlank_Refuses(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	if _, err := s.Add(t.Context(), owner, catalog.AddInput{Title: "   "}); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("a blank title was accepted, err = %v", err)
	}
}

func TestAdd_WhenTheCallerIsAReader_Refuses(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")
	reader := owner
	reader.Role = scope.RoleReader

	_, err := s.Add(t.Context(), reader, catalog.AddInput{Title: "The Hobbit"})
	if !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("a reader added a book, err = %v", err)
	}
}

func TestAdd_WhenTheCallerIsAnonymous_Refuses(t *testing.T) {
	s, _ := newStore(t)

	_, err := s.Add(t.Context(), scope.Caller{}, catalog.AddInput{Title: "The Hobbit"})
	if !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("an anonymous caller added a book, err = %v", err)
	}
}

func TestList_DoesNotLeakAnotherHousehold(t *testing.T) {
	s, db := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")

	if _, err := s.Add(t.Context(), alpha, catalog.AddInput{Title: "Alpha's book"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	books, err := s.List(t.Context(), beta, catalog.Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(books) != 0 {
		t.Fatalf("a member of another household saw %d books: %+v", len(books), books)
	}

	own, err := s.List(t.Context(), alpha, catalog.Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(own) != 1 {
		t.Errorf("the owning household sees %d books, want 1", len(own))
	}
}

func TestList_WhenTheCallerIsAnonymous_Refuses(t *testing.T) {
	s, _ := newStore(t)

	if _, err := s.List(t.Context(), scope.Caller{}, catalog.Query{}); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("an anonymous caller listed a catalogue, err = %v", err)
	}
}

func TestAdd_TheSameISBNInTwoHouseholds_StaysSeparate(t *testing.T) {
	s, db := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")

	in := catalog.AddInput{Title: "The Hobbit", ISBN: "9780140328721"}
	a, err := s.Add(t.Context(), alpha, in)
	if err != nil {
		t.Fatalf("alpha Add: %v", err)
	}
	b, err := s.Add(t.Context(), beta, in)
	if err != nil {
		t.Fatalf("beta Add: %v", err)
	}
	if a.WorkID == b.WorkID {
		t.Error("two households share a work row: the reference data is " +
			"duplicated per library on purpose")
	}
}
