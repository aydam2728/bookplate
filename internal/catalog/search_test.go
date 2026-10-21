package catalog_test

import (
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/catalog"
	"github.com/aydam2728/bookplate/internal/places"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// titles returns what a listing found, for an assertion that reads.
func titles(books []catalog.Book) []string {
	out := make([]string, 0, len(books))
	for _, b := range books {
		out = append(out, b.Title)
	}
	return out
}

func TestList_SearchesWhatAPersonRemembers(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	if _, err := s.Add(t.Context(), owner, catalog.AddInput{
		Title: "Étagère de verre", Author: "Jean Dupont", ISBN: "9780140328721",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(t.Context(), owner, catalog.AddInput{
		Title: "The Hobbit", Author: "J. R. R. Tolkien",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, c := range []struct {
		text string
		want int
	}{
		{"", 2},
		// The accents are the point: a French catalogue searched with an
		// ASCII keyboard has to find its own books.
		{"etagere", 1},
		{"ÉTAGÈRE", 1},
		{"verre dupont", 1},
		{"978-0-14-032872-1", 1},
		{"tolkien", 1},
		{"hobbit etagere", 0},
	} {
		books, err := s.List(t.Context(), owner, catalog.Query{Text: c.text})
		if err != nil {
			t.Fatalf("List(%q): %v", c.text, err)
		}
		if len(books) != c.want {
			t.Errorf("List(%q) found %v, want %d book(s)", c.text, titles(books), c.want)
		}
	}
}

func TestList_FiltersByPlaceSubtree(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")
	tree := places.NewStore(db)

	house, err := tree.Create(t.Context(), owner, "", "Maison", "building")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	shelf, err := tree.Create(t.Context(), owner, house.ID, "Étagère 3", "shelf")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	garage, err := tree.Create(t.Context(), owner, "", "Garage", "building")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, book := range []catalog.AddInput{
		{Title: "On the shelf", PlaceID: shelf.ID},
		{Title: "In the garage", PlaceID: garage.ID},
		{Title: "Nowhere yet"},
	} {
		if _, err := s.Add(t.Context(), owner, book); err != nil {
			t.Fatalf("Add %q: %v", book.Title, err)
		}
	}

	// The house holds the shelf, so asking for the house has to find what
	// sits on it. A filter that only matched the place itself would answer
	// "nothing is in the house", which is how a person stops trusting it.
	for _, c := range []struct {
		name  string
		query catalog.Query
		want  []string
	}{
		{"the house, which contains the shelf", catalog.Query{PlaceID: house.ID},
			[]string{"On the shelf"}},
		{"the shelf itself", catalog.Query{PlaceID: shelf.ID},
			[]string{"On the shelf"}},
		{"another branch of the tree", catalog.Query{PlaceID: garage.ID},
			[]string{"In the garage"}},
		{"the copies that sit nowhere", catalog.Query{Unfiled: true},
			[]string{"Nowhere yet"}},
	} {
		books, err := s.List(t.Context(), owner, c.query)
		if err != nil {
			t.Fatalf("List(%s): %v", c.name, err)
		}
		got := titles(books)
		if len(got) != len(c.want) || (len(got) == 1 && got[0] != c.want[0]) {
			t.Errorf("List(%s) found %v, want %v", c.name, got, c.want)
		}
	}
}

func TestList_PlaceFilterStopsAtTheHousehold(t *testing.T) {
	s, db := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")

	shelf, err := places.NewStore(db).Create(t.Context(), alpha, "", "Theirs", "shelf")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Add(t.Context(), alpha, catalog.AddInput{
		Title: "Theirs alone", PlaceID: shelf.ID,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Another household's place identifier is a guess away. It must return
	// nothing rather than their books.
	books, err := s.List(t.Context(), beta, catalog.Query{PlaceID: shelf.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("a neighbour's place returned %v, want nothing", titles(books))
	}
}

func TestList_RefusesAnImpossibleQuery(t *testing.T) {
	s, db := newStore(t)
	owner := household(t, db, "alpha")

	long := make([]byte, catalog.MaxQueryText+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := s.List(t.Context(), owner, catalog.Query{Text: string(long)}); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("a search of %d characters: %v, want ErrInvalid", len(long), err)
	}

	if _, err := s.List(t.Context(), owner, catalog.Query{
		PlaceID: "somewhere", Unfiled: true,
	}); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("a place and \"not filed\" together: %v, want ErrInvalid", err)
	}

	if _, err := s.List(t.Context(), scope.Caller{}, catalog.Query{}); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("an incomplete caller: %v, want ErrForbidden", err)
	}
}
