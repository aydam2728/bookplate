package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InventaireHost is the only host this provider may reach.
const InventaireHost = "inventaire.io"

// inventairePace spaces requests out. Their robots.txt asks crawlers for ten
// seconds between pages; this is their documented API rather than a page
// crawl, and a ten second wait would make a lookup unusable, so one second
// is the compromise.
const inventairePace = time.Second

// Wikidata property identifiers, which is how Inventaire stores everything.
const (
	propTitle     = "wdt:P1476"
	propPublished = "wdt:P577"
	propPublisher = "wdt:P123"
	propWork      = "wdt:P629"
	propAuthor    = "wdt:P50"
	propISBN13    = "wdt:P212"
	propISBN10    = "wdt:P957"
	propPages     = "wdt:P1104"
)

// Inventaire reads inventaire.io, an open book database built on Wikidata.
//
// It sits second in the chain: its data is open, it needs no key, its lookup
// by ISBN is exact rather than fuzzy, and unlike the national library it
// carries cover images.
type Inventaire struct {
	fetch getter
	base  string

	mu       sync.Mutex
	lastCall time.Time
}

// NewInventaire returns a provider fetching through f.
func NewInventaire(f getter) *Inventaire {
	return &Inventaire{fetch: f, base: "https://" + InventaireHost}
}

// Name identifies this provider in stored provenance.
func (i *Inventaire) Name() string { return "inventaire" }

// ByISBN looks one ISBN up.
//
// One request, not four: the relatives parameter pulls the work, its authors
// and the publisher back with the edition, which would otherwise be three
// more round trips for one book.
func (i *Inventaire) ByISBN(ctx context.Context, isbn13 string) (Record, error) {
	i.pace()

	target := i.base + "/api/entities?" + url.Values{
		"action":    {"by-uris"},
		"uris":      {"isbn:" + isbn13},
		"relatives": {strings.Join([]string{propWork, propAuthor, propPublisher}, "|")},
	}.Encode()

	body, err := i.fetch.Get(ctx, target)
	if err != nil {
		return Record{}, fmt.Errorf("inventaire: %w", err)
	}

	var answer invAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return Record{}, fmt.Errorf("inventaire sent something unreadable: %w", err)
	}

	edition, ok := answer.edition()
	if !ok {
		return Record{}, ErrNotFound
	}
	// Defensive rather than corrective: their lookup is exact, unlike the
	// national library's fuzzy index. Checking costs nothing and means a
	// future change upstream cannot quietly file the wrong book.
	if !matchesISBN(edition, isbn13) {
		return Record{}, ErrNotFound
	}

	record := Record{
		ISBN13:    isbn13,
		Title:     firstClaim(edition, propTitle),
		Published: firstClaim(edition, propPublished),
		PageCount: intClaim(edition, propPages),
		CoverURL:  i.coverURL(edition),
		Provider:  i.Name(),
	}
	if record.Title == "" {
		record.Title = clean(answer.labelOf(edition))
	}
	if publisher, ok := answer.related(edition, propPublisher); ok {
		record.Publisher = clean(answer.labelOf(publisher))
	}
	if work, ok := answer.related(edition, propWork); ok {
		for _, uri := range work.claimStrings(propAuthor) {
			if author, ok := answer.Entities[uri]; ok {
				if name := clean(answer.labelOf(author)); name != "" {
					record.Authors = append(record.Authors, name)
				}
			}
		}
	}
	return record, nil
}

func (i *Inventaire) pace() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if wait := inventairePace - time.Since(i.lastCall); wait > 0 {
		time.Sleep(wait)
	}
	i.lastCall = time.Now()
}

// coverURL turns the relative path Inventaire returns into an absolute one.
func (i *Inventaire) coverURL(e invEntity) string {
	path := strings.TrimSpace(e.Image.URL)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return i.base + "/" + strings.TrimPrefix(path, "/")
}

type invAnswer struct {
	Entities map[string]invEntity `json:"entities"`
}

type invEntity struct {
	URI    string                       `json:"uri"`
	Type   string                       `json:"type"`
	Labels map[string]string            `json:"labels"`
	Claims map[string][]json.RawMessage `json:"claims"`
	Image  struct {
		URL string `json:"url"`
	} `json:"image"`
}

// edition returns the one entity that is the book itself. The answer also
// carries the work, its authors and the publisher, which are not it.
func (a invAnswer) edition() (invEntity, bool) {
	for _, e := range a.Entities {
		if e.Type == "edition" {
			return e, true
		}
	}
	return invEntity{}, false
}

// related follows a claim that points at another entity in the same answer.
func (a invAnswer) related(e invEntity, property string) (invEntity, bool) {
	for _, uri := range e.claimStrings(property) {
		if found, ok := a.Entities[uri]; ok {
			return found, true
		}
	}
	return invEntity{}, false
}

// labelOf picks a readable name out of a multilingual map.
//
// French first, then English, then whatever there is: this catalogue will
// mostly hold French books, and a publisher labelled only in Ukrainian is
// still better than an empty field.
func (a invAnswer) labelOf(e invEntity) string {
	for _, key := range []string{"fromclaims", "fr", "en"} {
		if label, ok := e.Labels[key]; ok && strings.TrimSpace(label) != "" {
			return label
		}
	}
	for _, label := range e.Labels {
		if strings.TrimSpace(label) != "" {
			return label
		}
	}
	return ""
}

// claimStrings returns the string values of a claim. Claim values are not
// all strings, so anything that is not one is skipped rather than guessed at.
func (e invEntity) claimStrings(property string) []string {
	var out []string
	for _, raw := range e.Claims[property] {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			out = append(out, value)
		}
	}
	return out
}

func firstClaim(e invEntity, property string) string {
	values := e.claimStrings(property)
	if len(values) == 0 {
		return ""
	}
	return clean(values[0])
}

func intClaim(e invEntity, property string) int {
	for _, raw := range e.Claims[property] {
		var number float64
		if err := json.Unmarshal(raw, &number); err == nil {
			return int(number)
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
				return n
			}
		}
	}
	return 0
}

// matchesISBN reports whether the edition is the book that was asked for.
//
// The request names an exact URI, isbn:<the thirteen digits>, so resolving at
// all is already evidence. This guard only catches the case that would be
// plainly wrong: a record carrying a different ISBN-13. A record that stores
// no ISBN-13 at all cannot be checked here and is accepted on the strength of
// the lookup itself, rather than converting an ISBN-10 with a checksum this
// package would then have to keep in step with the catalogue's.
func matchesISBN(e invEntity, want string) bool {
	claimed := e.claimStrings(propISBN13)
	if len(claimed) == 0 {
		return true
	}
	for _, value := range claimed {
		if digitsOf(value) == want {
			return true
		}
	}
	return false
}
