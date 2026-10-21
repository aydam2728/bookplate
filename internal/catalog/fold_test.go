package catalog

import "testing"

func TestFold(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Étagère 3", "etagere 3"},
		{"ÉTAGÈRE", "etagere"},
		// The same word typed decomposed: "e" followed by a combining acute.
		{"Étagère", "etagere"},
		{"Cœur d’Alene", "coeur d'alene"},
		{"Straße", "strasse"},
		{"Æon", "aeon"},
		{"L’Œuvre", "l'oeuvre"},
		{"Žižek", "zizek"},
		// Nothing to fold, nothing changed.
		{"The Hobbit", "the hobbit"},
		// A script the table says nothing about has to survive intact, or
		// the book becomes unfindable.
		{"百年の孤独", "百年の孤独"},
		{"", ""},
	} {
		if got := fold(c.in); got != c.want {
			t.Errorf("fold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDigits(t *testing.T) {
	if got := digits("978-0-14-032872-1"); got != "9780140328721" {
		t.Errorf("digits = %q, want the ISBN without its hyphens", got)
	}
	if got := digits("no numbers here"); got != "" {
		t.Errorf("digits = %q, want empty", got)
	}
}

func TestMatches(t *testing.T) {
	book := Book{
		Title:     "Étagère de verre",
		Authors:   "Jean Dupont",
		Publisher: "Gallimard",
		Published: "1998",
		PlaceName: "Salon",
		Summary:   "Celui avec le blaireau.",
		Notes:     "Dédicacé à Lyon.",
		ISBN13:    "9780140328721",
	}

	for _, c := range []struct {
		text string
		want bool
	}{
		{"", true},
		{"etagere", true},                  // typed without the accents
		{"ÉTAGÈRE", true},                  // and with them, in capitals
		{"dupont etagere", true},           // two words, wrong order
		{"blaireau", true},                 // from the summary the household wrote
		{"lyon", true},                     // from the notes on this copy
		{"salon", true},                    // from where it sits
		{"9780140328721", true},            // the barcode as scanned
		{"978-0-14-032872-1", true},        // and as printed on the cover
		{"etagere dupont gallimard", true}, // every word has to appear
		{"etagere tolkien", false},         // and one that does not fails it
		{"9780140328722", false},           // a neighbouring ISBN is not it
	} {
		if got := matches(book, words(c.text)); got != c.want {
			t.Errorf("matches(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
