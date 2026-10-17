package id_test

import (
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/id"
)

func TestNew_HasTheExpectedShape(t *testing.T) {
	got, err := id.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(got) != id.Length {
		t.Errorf("length %d, want %d: %q", len(got), id.Length, got)
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, r := range got {
		if !strings.ContainsRune(alphabet, r) {
			t.Errorf("%q contains %q, which is not in the Crockford alphabet", got, r)
		}
	}
}

func TestNew_IsUnique(t *testing.T) {
	const runs = 10000
	seen := make(map[string]bool, runs)
	for i := 0; i < runs; i++ {
		got, err := id.New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if seen[got] {
			t.Fatalf("%q was generated twice in %d draws", got, runs)
		}
		seen[got] = true
	}
}

func TestNew_SortsChronologically(t *testing.T) {
	// Identifiers minted later must sort after earlier ones, which is what
	// keeps index locality in SQLite.
	first, err := id.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var later string
	for {
		later, err = id.New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// Wait until the millisecond ticks over, otherwise the random part
		// decides the order and the comparison means nothing.
		if later[:10] != first[:10] {
			break
		}
	}
	if later <= first {
		t.Errorf("%q was minted after %q but does not sort after it", later, first)
	}
}
