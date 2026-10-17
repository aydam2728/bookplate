package metadata_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/metadata"
)

// stub replays a recorded answer. No test in this package touches the
// network: a suite that depends on someone else's uptime goes red for reasons
// that have nothing to do with the change under review, and people learn to
// ignore red.
type stub struct {
	body    string
	err     error
	lastURL string
}

func (s *stub) Get(_ context.Context, target string) ([]byte, error) {
	s.lastURL = target
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.body), nil
}

// recorded is the shape openlibrary.org returns for jscmd=data, trimmed to
// the fields this project reads.
const recorded = `{
  "ISBN:9780140328721": {
    "title": "The Hobbit",
    "subtitle": "or There and Back Again",
    "publish_date": "1998",
    "number_of_pages": 279,
    "authors": [{"name": "J. R. R. Tolkien"}],
    "publishers": [{"name": "Puffin"}],
    "cover": {"medium": "https://covers.openlibrary.org/b/id/1-M.jpg",
              "large": "https://covers.openlibrary.org/b/id/1-L.jpg"}
  }
}`

func TestOpenLibrary_ByISBN_ReadsARecord(t *testing.T) {
	s := &stub{body: recorded}
	got, err := metadata.NewOpenLibrary(s).ByISBN(t.Context(), "9780140328721")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}

	if got.Title != "The Hobbit" {
		t.Errorf("Title = %q", got.Title)
	}
	if len(got.Authors) != 1 || got.Authors[0] != "J. R. R. Tolkien" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if got.Publisher != "Puffin" {
		t.Errorf("Publisher = %q", got.Publisher)
	}
	if got.Published != "1998" || got.PageCount != 279 {
		t.Errorf("Published = %q, PageCount = %d", got.Published, got.PageCount)
	}
	if !strings.HasSuffix(got.CoverURL, "-L.jpg") {
		t.Errorf("CoverURL = %q, want the large one", got.CoverURL)
	}
	if got.Provider != "openlibrary" {
		t.Errorf("Provider = %q", got.Provider)
	}
	if !strings.Contains(s.lastURL, "openlibrary.org") {
		t.Errorf("asked %q, which is not Open Library", s.lastURL)
	}
}

func TestOpenLibrary_ByISBN_WhenNothingIsKnown_ReportsNotFound(t *testing.T) {
	// Most of what a household owns predates anyone cataloguing it online, so
	// this is an ordinary outcome rather than a failure.
	s := &stub{body: `{}`}
	_, err := metadata.NewOpenLibrary(s).ByISBN(t.Context(), "9780140328721")
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestOpenLibrary_ByISBN_StripsControlCharacters(t *testing.T) {
	// Escaped control characters are valid JSON, so they survive decoding and
	// reach the stored value. Provider data is written by the public.
	s := &stub{body: `{"ISBN:9780140328721": {"title": "  The\u0000 Hobbit\u0007  "}}`}
	got, err := metadata.NewOpenLibrary(s).ByISBN(t.Context(), "9780140328721")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}
	if got.Title != "The Hobbit" {
		t.Errorf("Title = %q, want %q", got.Title, "The Hobbit")
	}
}

func TestOpenLibrary_ByISBN_WhenTheAnswerIsNotJSON_Fails(t *testing.T) {
	s := &stub{body: "<html>down for maintenance</html>"}
	_, err := metadata.NewOpenLibrary(s).ByISBN(t.Context(), "9780140328721")
	if err == nil {
		t.Error("an HTML error page was accepted as a record")
	}
}

// fake is a provider with a fixed answer, for exercising the chain.
type fake struct {
	name   string
	record metadata.Record
	err    error
}

func (f fake) Name() string { return f.name }
func (f fake) ByISBN(context.Context, string) (metadata.Record, error) {
	return f.record, f.err
}

func TestChain_ReturnsTheFirstProviderThatKnows(t *testing.T) {
	chain := metadata.Chain{
		fake{name: "a", err: metadata.ErrNotFound},
		fake{name: "b", record: metadata.Record{Title: "Found here"}},
		fake{name: "c", record: metadata.Record{Title: "Never reached"}},
	}

	got, err := chain.ByISBN(t.Context(), "9780140328721")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}
	if got.Title != "Found here" {
		t.Errorf("Title = %q", got.Title)
	}
}

func TestChain_WhenOneProviderIsDown_StillUsesTheOthers(t *testing.T) {
	chain := metadata.Chain{
		fake{name: "down", err: errors.New("connection refused")},
		fake{name: "up", record: metadata.Record{Title: "Found anyway"}},
	}

	got, err := chain.ByISBN(t.Context(), "9780140328721")
	if err != nil {
		t.Fatalf("one provider being down hid a working one: %v", err)
	}
	if got.Title != "Found anyway" {
		t.Errorf("Title = %q", got.Title)
	}
}

func TestChain_WhenNobodyKnows_ReportsNotFound(t *testing.T) {
	chain := metadata.Chain{
		fake{name: "a", err: metadata.ErrNotFound},
		fake{name: "b", err: metadata.ErrNotFound},
	}

	if _, err := chain.ByISBN(t.Context(), "9780140328721"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestChain_WhenEveryProviderFails_ReportsWhy(t *testing.T) {
	chain := metadata.Chain{fake{name: "a", err: errors.New("timed out")}}

	_, err := chain.ByISBN(t.Context(), "9780140328721")
	if err == nil || errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("err = %v: a provider that timed out is not the same as a book "+
			"nobody has catalogued", err)
	}
}
