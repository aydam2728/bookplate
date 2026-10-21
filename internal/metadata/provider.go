package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotFound reports that a provider has no record for an ISBN. It is an
// ordinary outcome, not a failure: most of what a household owns predates
// anyone cataloguing it online.
var ErrNotFound = errors.New("no record for this ISBN")

// Record is what a provider found. Fields it could not fill are empty, and a
// caller must treat all of it as untrusted text: these catalogues are edited
// by the public.
type Record struct {
	ISBN13    string   `json:"isbn13"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle,omitempty"`
	Authors   []string `json:"authors,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Published string   `json:"published,omitempty"`
	PageCount int      `json:"pageCount,omitempty"`
	CoverURL  string   `json:"coverUrl,omitempty"`
	Provider  string   `json:"provider"`
}

// Provider looks an ISBN up in one external catalogue.
type Provider interface {
	Name() string
	ByISBN(ctx context.Context, isbn13 string) (Record, error)
}

// getter is the one thing a provider needs from the network, which lets a
// test drive a provider without any network at all.
type getter interface {
	Get(ctx context.Context, target string) ([]byte, error)
}

// OpenLibraryHost is the only host the Open Library provider may reach.
const OpenLibraryHost = "openlibrary.org"

// openLibraryPace keeps requests under the documented limit. Identified
// callers get three a second; one a second leaves room to be a good citizen.
const openLibraryPace = time.Second

// OpenLibrary reads https://openlibrary.org, whose data is in the public
// domain, which is why it is the default and the only one enabled without an
// operator opting in.
type OpenLibrary struct {
	fetch getter
	base  string

	mu       sync.Mutex
	lastCall time.Time
}

// NewOpenLibrary returns a provider fetching through f.
func NewOpenLibrary(f getter) *OpenLibrary {
	return &OpenLibrary{fetch: f, base: "https://" + OpenLibraryHost}
}

// Name identifies this provider in stored provenance.
func (o *OpenLibrary) Name() string { return "openlibrary" }

// ByISBN looks one ISBN up. One request, not two: the books endpoint already
// carries the author names, where the record endpoint only carries keys.
func (o *OpenLibrary) ByISBN(ctx context.Context, isbn13 string) (Record, error) {
	o.pace()

	key := "ISBN:" + isbn13
	target := o.base + "/api/books?" + url.Values{
		"bibkeys": {key},
		"format":  {"json"},
		"jscmd":   {"data"},
	}.Encode()

	body, err := o.fetch.Get(ctx, target)
	if err != nil {
		return Record{}, fmt.Errorf("open library: %w", err)
	}

	var payload map[string]openLibraryBook
	if err := json.Unmarshal(body, &payload); err != nil {
		return Record{}, fmt.Errorf("open library sent something unreadable: %w", err)
	}
	book, ok := payload[key]
	if !ok || strings.TrimSpace(book.Title) == "" {
		return Record{}, ErrNotFound
	}

	record := Record{
		ISBN13:    isbn13,
		Title:     clean(book.Title),
		Subtitle:  clean(book.Subtitle),
		Published: clean(book.PublishDate),
		PageCount: book.NumberOfPages,
		CoverURL:  book.Cover.Large,
		Provider:  o.Name(),
	}
	for _, a := range book.Authors {
		if name := clean(a.Name); name != "" {
			record.Authors = append(record.Authors, name)
		}
	}
	if len(book.Publishers) > 0 {
		record.Publisher = clean(book.Publishers[0].Name)
	}
	if record.CoverURL == "" {
		record.CoverURL = book.Cover.Medium
	}
	return record, nil
}

// pace spaces requests out. A bulk scan would otherwise fire as fast as
// someone can pull books off a shelf, and being rate-limited mid-session is a
// worse experience than waiting a second.
func (o *OpenLibrary) pace() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if wait := openLibraryPace - time.Since(o.lastCall); wait > 0 {
		time.Sleep(wait)
	}
	o.lastCall = time.Now()
}

type openLibraryBook struct {
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle"`
	PublishDate   string `json:"publish_date"`
	NumberOfPages int    `json:"number_of_pages"`
	Authors       []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	Cover struct {
		Medium string `json:"medium"`
		Large  string `json:"large"`
	} `json:"cover"`
}

// clean trims a value and drops control characters. Provider data is written
// by the public and is treated as text everywhere, never as markup.
func clean(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s))
}

// Chain asks each provider in turn and returns the first record found.
type Chain []Provider

// ByISBN returns the first record any provider has, or ErrNotFound.
//
// A record that arrives without a cover is completed from the rest of the
// chain, because the catalogue that knows a book is often not the one that
// has a picture of it. The BnF catalogues every book published in France and
// publishes no cover at all, so without this a French novel arrives as a
// line of text.
func (c Chain) ByISBN(ctx context.Context, isbn13 string) (Record, error) {
	var failures []error
	for i, p := range c {
		record, err := p.ByISBN(ctx, isbn13)
		switch {
		case err == nil:
			if record.CoverURL == "" {
				record.CoverURL = c.coverAfter(ctx, isbn13, i)
			}
			return record, nil
		case errors.Is(err, ErrNotFound):
			continue
		default:
			// One provider being down must not hide another that works.
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return Record{}, errors.Join(failures...)
	}
	return Record{}, ErrNotFound
}

// coverAfter asks the providers past the one that answered for a picture and
// nothing else. The ones before it have already said they do not have the
// book, so asking them again would buy nothing and cost a request each.
//
// This runs only when the winning record has no cover, so a lookup that
// comes back complete still costs exactly one request. A provider that fails
// here is ignored: the book is already in hand, and a missing picture is not
// a reason to refuse it.
func (c Chain) coverAfter(ctx context.Context, isbn13 string, answered int) string {
	for _, p := range c[answered+1:] {
		record, err := p.ByISBN(ctx, isbn13)
		if err == nil && record.CoverURL != "" {
			return record.CoverURL
		}
	}
	return ""
}
