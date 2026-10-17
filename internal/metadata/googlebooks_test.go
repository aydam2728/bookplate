package metadata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/metadata"
)

// recordedGoogle is the shape www.googleapis.com returns, trimmed to the
// fields this project reads. The thumbnail really does come back as plain
// HTTP.
const recordedGoogle = `{
  "totalItems": 1,
  "items": [{
    "volumeInfo": {
      "title": "Les Travailleurs de la mer",
      "subtitle": "roman",
      "authors": ["Victor Hugo"],
      "publisher": "Gallimard",
      "publishedDate": "2021-03-04",
      "pageCount": 672,
      "imageLinks": {
        "thumbnail": "http://books.google.com/books/content?id=abc&img=1"
      }
    }
  }]
}`

func TestNewGoogleBooks_WithoutAKey_IsNotBuilt(t *testing.T) {
	// Without a key the API answers 429 from a shared anonymous quota that is
	// routinely exhausted, so an instance with no key is better off not
	// asking at all.
	for _, key := range []string{"", "   "} {
		if p := metadata.NewGoogleBooks(&stub{}, key); p != nil {
			t.Errorf("a provider was built with key %q", key)
		}
	}
}

func TestGoogleBooks_ByISBN_ReadsARecord(t *testing.T) {
	s := &stub{body: recordedGoogle}
	got, err := metadata.NewGoogleBooks(s, "a-key").ByISBN(t.Context(), "9782072903434")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}

	if got.Title != "Les Travailleurs de la mer" {
		t.Errorf("Title = %q", got.Title)
	}
	if len(got.Authors) != 1 || got.Authors[0] != "Victor Hugo" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if got.Publisher != "Gallimard" || got.Published != "2021-03-04" || got.PageCount != 672 {
		t.Errorf("got %+v", got)
	}
	if got.Provider != "google" {
		t.Errorf("Provider = %q", got.Provider)
	}
}

func TestGoogleBooks_ByISBN_UpgradesThePlainHTTPThumbnail(t *testing.T) {
	s := &stub{body: recordedGoogle}
	got, err := metadata.NewGoogleBooks(s, "a-key").ByISBN(t.Context(), "9782072903434")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}
	if !strings.HasPrefix(got.CoverURL, "https://") {
		t.Errorf("CoverURL = %q: the outbound client refuses plain HTTP, and "+
			"fetching a cover in the clear is not worth working around", got.CoverURL)
	}
}

func TestGoogleBooks_ByISBN_WhenNothingIsKnown_ReportsNotFound(t *testing.T) {
	for _, body := range []string{`{"totalItems": 0}`, `{"items": []}`,
		`{"items":[{"volumeInfo":{"title":"  "}}]}`} {
		_, err := metadata.NewGoogleBooks(&stub{body: body}, "a-key").
			ByISBN(t.Context(), "9782072903434")
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Errorf("body %q gave err = %v, want ErrNotFound", body, err)
		}
	}
}

func TestGoogleBooks_ByISBN_AsksTheRightHostAndCarriesTheKey(t *testing.T) {
	s := &stub{body: recordedGoogle}
	if _, err := metadata.NewGoogleBooks(s, "secret-key").ByISBN(t.Context(), "9782072903434"); err != nil {
		t.Fatalf("ByISBN: %v", err)
	}
	if !strings.Contains(s.lastURL, metadata.GoogleBooksHost) {
		t.Errorf("asked %q, which is not Google Books", s.lastURL)
	}
	if !strings.Contains(s.lastURL, "isbn%3A9782072903434") && !strings.Contains(s.lastURL, "isbn:9782072903434") {
		t.Errorf("the ISBN is not in the query: %q", s.lastURL)
	}
	if !strings.Contains(s.lastURL, "secret-key") {
		t.Error("the key is missing from the request, so the call would be throttled")
	}
}

func TestGoogleBooks_ByISBN_WhenTheFetchFails_DoesNotLeakTheKey(t *testing.T) {
	s := &stub{err: errors.New("connection refused")}
	_, err := metadata.NewGoogleBooks(s, "super-secret-key").ByISBN(t.Context(), "9782072903434")
	if err == nil {
		t.Fatal("a failed fetch was reported as success")
	}
	if strings.Contains(err.Error(), "super-secret-key") {
		t.Errorf("the API key leaked into an error that will be logged: %v", err)
	}
}

func TestChain_WhenOpenLibraryDoesNotKnow_GoogleAnswers(t *testing.T) {
	// The whole point of the third provider: 9782072903434 is absent from
	// Open Library and present at Google.
	chain := metadata.Chain{
		metadata.NewOpenLibrary(&stub{body: `{}`}),
		metadata.NewGoogleBooks(&stub{body: recordedGoogle}, "a-key"),
	}

	got, err := chain.ByISBN(t.Context(), "9782072903434")
	if err != nil {
		t.Fatalf("the chain gave up: %v", err)
	}
	if got.Provider != "google" {
		t.Errorf("Provider = %q, want the fallback to have answered", got.Provider)
	}
}
