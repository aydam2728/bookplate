package catalog_test

import (
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/catalog"
)

func TestNormaliseISBN_Accepts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ISBN-13 plain", "9780140328721", "9780140328721"},
		{"ISBN-13 hyphenated", "978-0-14-032872-1", "9780140328721"},
		{"ISBN-13 spaced", "978 0 14 032872 1", "9780140328721"},
		{"ISBN-10 converted", "0140328726", "9780140328721"},
		{"ISBN-10 hyphenated", "0-14-032872-6", "9780140328721"},
		{"ISBN-10 with X check digit", "043942089X", "9780439420891"},
		{"979 prefix accepted", "9791032401880", "9791032401880"},
		{"price add-on ignored", "978014032872151234", "9780140328721"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := catalog.NormaliseISBN(tt.input)
			if err != nil {
				t.Fatalf("NormaliseISBN(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormaliseISBN_RejectsAMistypedIdentifier(t *testing.T) {
	// One digit changed. It must fail here, before any network call.
	for _, bad := range []string{"9780140328722", "0140328727"} {
		t.Run(bad, func(t *testing.T) {
			if _, err := catalog.NormaliseISBN(bad); !errors.Is(err, catalog.ErrBadISBN) {
				t.Errorf("a wrong check digit was accepted, err = %v", err)
			}
		})
	}
}

func TestNormaliseISBN_RejectsAPeriodical(t *testing.T) {
	// 977 is an ISSN. Scanning a magazine is an easy mistake and deserves a
	// message of its own.
	_, err := catalog.NormaliseISBN("9771234567003")
	if !errors.Is(err, catalog.ErrNotABook) {
		t.Errorf("a periodical barcode was accepted as a book, err = %v", err)
	}
}

func TestNormaliseISBN_RejectsOtherProducts(t *testing.T) {
	// A tin of beans has a valid EAN-13 that is not a Bookland prefix.
	if _, err := catalog.NormaliseISBN("4006381333931"); !errors.Is(err, catalog.ErrNotABook) {
		t.Errorf("a non-book EAN was accepted, err = %v", err)
	}
}

func TestNormaliseISBN_RejectsTheWrongLength(t *testing.T) {
	for _, bad := range []string{"", "123", "97801403287211"} {
		if _, err := catalog.NormaliseISBN(bad); !errors.Is(err, catalog.ErrBadISBN) {
			t.Errorf("%q was accepted, err = %v", bad, err)
		}
	}
}
