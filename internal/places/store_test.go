package places_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/places"
	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

func newStore(t *testing.T) (*places.Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return places.NewStore(db), db
}

func household(t *testing.T, db *sql.DB, name string) scope.Caller {
	t.Helper()
	const now = "2026-01-01T00:00:00Z"
	lib := name + "-lib"
	stmts := [][]any{
		{`INSERT INTO library (id, name, created_at) VALUES (?, ?, ?)`, lib, name, now},
		{`INSERT INTO work (id, library_id, title, created_at, updated_at)
		  VALUES (?, ?, 'A book', ?, ?)`, name + "-work", lib, now, now},
		{`INSERT INTO edition (id, library_id, work_id, created_at, updated_at)
		  VALUES (?, ?, ?, ?, ?)`, name + "-ed", lib, name + "-work", now, now},
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(t.Context(), q[0].(string), q[1:]...); err != nil {
			t.Fatalf("building the %s household: %v", name, err)
		}
	}
	return scope.Caller{UserID: name + "-user", LibraryID: lib, Role: scope.RoleOwner}
}

// addCopy files one copy into placeID; an empty placeID leaves it unfiled.
func addCopy(t *testing.T, db *sql.DB, caller scope.Caller, copyID, placeID string) {
	t.Helper()
	const now = "2026-01-01T00:00:00Z"
	var place any
	if placeID != "" {
		place = placeID
	}
	edition := strings.TrimSuffix(caller.LibraryID, "-lib") + "-ed"
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO copy (id, library_id, edition_id, place_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		copyID, caller.LibraryID, edition, place, now, now)
	if err != nil {
		t.Fatalf("adding a copy: %v", err)
	}
}

func copiesIn(t *testing.T, s *places.Store, caller scope.Caller, placeID string) int {
	t.Helper()
	all, err := s.List(t.Context(), caller)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range all {
		if p.ID == placeID {
			return p.Copies
		}
	}
	t.Fatalf("place %s is not in the tree", placeID)
	return 0
}

// tree builds House > Living room > Bookcase, and House > Garage.
func tree(t *testing.T, s *places.Store, c scope.Caller) (house, living, bookcase, garage places.Place) {
	t.Helper()
	var err error
	if house, err = s.Create(t.Context(), c, "", "House", "building"); err != nil {
		t.Fatalf("Create House: %v", err)
	}
	if living, err = s.Create(t.Context(), c, house.ID, "Living room", "room"); err != nil {
		t.Fatalf("Create Living room: %v", err)
	}
	if bookcase, err = s.Create(t.Context(), c, living.ID, "Bookcase", "furniture"); err != nil {
		t.Fatalf("Create Bookcase: %v", err)
	}
	if garage, err = s.Create(t.Context(), c, house.ID, "Garage", "room"); err != nil {
		t.Fatalf("Create Garage: %v", err)
	}
	return house, living, bookcase, garage
}

func TestCreate_BuildsPathsAndDepths(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	house, living, bookcase, _ := tree(t, s, c)

	if house.Depth != 0 || living.Depth != 1 || bookcase.Depth != 2 {
		t.Errorf("depths = %d, %d, %d; want 0, 1, 2", house.Depth, living.Depth, bookcase.Depth)
	}
	if !strings.HasPrefix(bookcase.Path, house.Path) {
		t.Errorf("the bookcase path %q does not start with the house path %q",
			bookcase.Path, house.Path)
	}
	if !strings.HasSuffix(bookcase.Path, bookcase.ID+"/") {
		t.Errorf("the path %q does not end with the place's own id", bookcase.Path)
	}
}

func TestCreate_AcceptsAnyDepth(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")

	parent := ""
	for i := 0; i < 8; i++ {
		p, err := s.Create(t.Context(), c, parent, "level", "other")
		if err != nil {
			t.Fatalf("level %d refused: %v", i, err)
		}
		if p.Depth != i {
			t.Fatalf("level %d has depth %d", i, p.Depth)
		}
		parent = p.ID
	}
}

func TestList_PutsParentsBeforeChildren(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	house, _, bookcase, _ := tree(t, s, c)

	all, err := s.List(t.Context(), c)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	seen := map[string]int{}
	for i, p := range all {
		seen[p.ID] = i
	}
	if seen[house.ID] > seen[bookcase.ID] {
		t.Error("a child was listed before its parent, so a tree cannot be drawn in one pass")
	}
}

func TestCopies_CountTowardsEveryAncestor(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	house, living, bookcase, garage := tree(t, s, c)

	addCopy(t, db, c, "c1", bookcase.ID)
	addCopy(t, db, c, "c2", garage.ID)

	if got := copiesIn(t, s, c, bookcase.ID); got != 1 {
		t.Errorf("bookcase holds %d, want 1", got)
	}
	if got := copiesIn(t, s, c, living.ID); got != 1 {
		t.Errorf("living room holds %d, want 1: a copy counts for every ancestor", got)
	}
	if got := copiesIn(t, s, c, house.ID); got != 2 {
		t.Errorf("house holds %d, want 2", got)
	}
}

func TestMove_RewritesTheWholeSubtree(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	_, living, bookcase, garage := tree(t, s, c)
	addCopy(t, db, c, "c1", bookcase.ID)

	if err := s.Move(t.Context(), c, living.ID, garage.ID); err != nil {
		t.Fatalf("Move: %v", err)
	}

	all, err := s.List(t.Context(), c)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byID := map[string]places.Place{}
	for _, p := range all {
		byID[p.ID] = p
	}
	if byID[living.ID].Depth != 2 || byID[bookcase.ID].Depth != 3 {
		t.Errorf("depths after the move: living %d, bookcase %d; want 2 and 3",
			byID[living.ID].Depth, byID[bookcase.ID].Depth)
	}
	if !strings.HasPrefix(byID[bookcase.ID].Path, byID[garage.ID].Path) {
		t.Error("the grandchild's path was not rewritten, so it is orphaned")
	}
	if got := byID[garage.ID].Copies; got != 1 {
		t.Errorf("the garage holds %d after the move, want 1: the counters must "+
			"follow the subtree", got)
	}
}

func TestMove_RefusesToMakeAPlaceItsOwnAncestor(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	_, living, bookcase, _ := tree(t, s, c)

	if err := s.Move(t.Context(), c, living.ID, bookcase.ID); !errors.Is(err, places.ErrWouldCycle) {
		t.Errorf("moving a place inside its own child was accepted, err = %v", err)
	}
	if err := s.Move(t.Context(), c, living.ID, living.ID); !errors.Is(err, places.ErrWouldCycle) {
		t.Errorf("moving a place into itself was accepted, err = %v", err)
	}
}

func TestAssignCopies_MovesManyAtOnce(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	_, _, bookcase, garage := tree(t, s, c)
	for _, one := range []string{"c1", "c2", "c3"} {
		addCopy(t, db, c, one, bookcase.ID)
	}

	moved, err := s.AssignCopies(t.Context(), c, []string{"c1", "c2", "c3"}, garage.ID)
	if err != nil {
		t.Fatalf("AssignCopies: %v", err)
	}
	if moved != 3 {
		t.Errorf("moved %d, want 3", moved)
	}
	if got := copiesIn(t, s, c, bookcase.ID); got != 0 {
		t.Errorf("the bookcase still holds %d", got)
	}
	if got := copiesIn(t, s, c, garage.ID); got != 3 {
		t.Errorf("the garage holds %d, want 3", got)
	}
}

func TestAssignCopies_DoesNotTouchAnotherHousehold(t *testing.T) {
	s, db := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")
	_, _, bookcase, _ := tree(t, s, alpha)
	addCopy(t, db, alpha, "c1", bookcase.ID)

	theirs, err := s.Create(t.Context(), beta, "", "Their shelf", "shelf")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	moved, err := s.AssignCopies(t.Context(), beta, []string{"c1"}, theirs.ID)
	if err != nil {
		t.Fatalf("AssignCopies: %v", err)
	}
	if moved != 0 {
		t.Error("a member of another household re-shelved someone else's copy")
	}
}

func TestDelete_UnfilesCopiesRatherThanLosingThem(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	_, _, bookcase, _ := tree(t, s, c)
	addCopy(t, db, c, "c1", bookcase.ID)

	if err := s.Delete(t.Context(), c, bookcase.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM copy WHERE id = 'c1' AND deleted_at IS NULL`).Scan(&count)
	if err != nil {
		t.Fatalf("counting the copy: %v", err)
	}
	if count != 1 {
		t.Fatal("deleting a shelf deleted the book that was on it")
	}
}

func TestDelete_RefusesANodeThatStillHasPlacesInside(t *testing.T) {
	s, db := newStore(t)
	c := household(t, db, "alpha")
	_, living, _, _ := tree(t, s, c)

	if err := s.Delete(t.Context(), c, living.ID); !errors.Is(err, places.ErrInvalid) {
		t.Errorf("a place with children was deleted, err = %v", err)
	}
}

func TestList_DoesNotLeakAnotherHousehold(t *testing.T) {
	s, db := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")
	tree(t, s, alpha)

	all, err := s.List(t.Context(), beta)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("another household saw %d places", len(all))
	}
}

func TestCreate_WhenTheCallerIsAReader_Refuses(t *testing.T) {
	s, db := newStore(t)
	reader := household(t, db, "alpha")
	reader.Role = scope.RoleReader

	if _, err := s.Create(t.Context(), reader, "", "Shelf", "shelf"); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("a reader created a place, err = %v", err)
	}
}
