package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GoogleBooksHost is the only host this provider may reach.
const GoogleBooksHost = "www.googleapis.com"

// GoogleThumbnailHost serves the cover thumbnails Google returns. It is a
// different host from the catalogue and has to be allowed separately.
const GoogleThumbnailHost = "books.google.com"

// googleBooksPace spaces requests out. The quota is generous with a key but
// not unlimited, and a bulk scan should not burn a day of it in a minute.
const googleBooksPace = 500 * time.Millisecond

// GoogleBooks reads the Google Books API.
//
// It exists because Open Library does not know everything: a good deal of
// recent publishing, French in particular, is missing there and present here.
// It is the third link in the chain, not the first, because its terms are
// more restrictive than Open Library's public domain data.
type GoogleBooks struct {
	fetch getter
	key   string
	base  string

	mu       sync.Mutex
	lastCall time.Time
}

// NewGoogleBooks returns a provider, or nil when no key is configured.
//
// A nil provider rather than one that fails on every call: without a key the
// API answers 429 from a shared anonymous quota that is routinely exhausted,
// so an instance with no key is better off not asking at all.
func NewGoogleBooks(f getter, key string) *GoogleBooks {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	return &GoogleBooks{fetch: f, key: strings.TrimSpace(key), base: "https://" + GoogleBooksHost}
}

// Name identifies this provider in stored provenance.
func (g *GoogleBooks) Name() string { return "google" }

// ByISBN looks one ISBN up.
func (g *GoogleBooks) ByISBN(ctx context.Context, isbn13 string) (Record, error) {
	g.pace()

	target := g.base + "/books/v1/volumes?" + url.Values{
		"q":          {"isbn:" + isbn13},
		"country":    {"FR"},
		"maxResults": {"1"},
		"key":        {g.key},
	}.Encode()

	body, err := g.fetch.Get(ctx, target)
	if err != nil {
		// Deliberately not quoting the URL: it carries the API key.
		return Record{}, fmt.Errorf("google books: %w", err)
	}

	var payload googleBooksResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Record{}, fmt.Errorf("google books sent something unreadable: %w", err)
	}
	if len(payload.Items) == 0 {
		return Record{}, ErrNotFound
	}

	info := payload.Items[0].VolumeInfo
	if strings.TrimSpace(info.Title) == "" {
		return Record{}, ErrNotFound
	}

	record := Record{
		ISBN13:    isbn13,
		Title:     clean(info.Title),
		Subtitle:  clean(info.Subtitle),
		Publisher: clean(info.Publisher),
		Published: clean(info.PublishedDate),
		PageCount: info.PageCount,
		CoverURL:  secureThumbnail(info.ImageLinks.Thumbnail),
		Provider:  g.Name(),
	}
	for _, a := range info.Authors {
		if name := clean(a); name != "" {
			record.Authors = append(record.Authors, name)
		}
	}
	return record, nil
}

func (g *GoogleBooks) pace() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if wait := googleBooksPace - time.Since(g.lastCall); wait > 0 {
		time.Sleep(wait)
	}
	g.lastCall = time.Now()
}

// secureThumbnail upgrades the plain-HTTP thumbnail URL Google returns. The
// outbound client refuses plain HTTP, and fetching a cover over a cleartext
// connection is not something to work around.
func secureThumbnail(raw string) string {
	if raw == "" {
		return ""
	}
	return strings.Replace(raw, "http://", "https://", 1)
}

type googleBooksResponse struct {
	Items []struct {
		VolumeInfo struct {
			Title         string   `json:"title"`
			Subtitle      string   `json:"subtitle"`
			Authors       []string `json:"authors"`
			Publisher     string   `json:"publisher"`
			PublishedDate string   `json:"publishedDate"`
			PageCount     int      `json:"pageCount"`
			ImageLinks    struct {
				Thumbnail string `json:"thumbnail"`
			} `json:"imageLinks"`
		} `json:"volumeInfo"`
	} `json:"items"`
}
