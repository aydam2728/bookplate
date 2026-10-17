package metadata

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BnFHost is the only host this provider may reach.
const BnFHost = "catalogue.bnf.fr"

// bnfPace spaces requests out. The service is a public good with no key and
// no published quota; hammering it would be a poor way to repay that.
const bnfPace = 500 * time.Millisecond

// BnF reads the French national library's SRU catalogue.
//
// It sits second in the chain because French legal deposit makes it
// authoritative for anything published in France, which is most of what a
// French household owns and a large part of what Open Library has never
// heard of. It needs no key and no quota.
type BnF struct {
	fetch getter
	base  string

	mu       sync.Mutex
	lastCall time.Time
}

// NewBnF returns a provider fetching through f.
func NewBnF(f getter) *BnF {
	return &BnF{fetch: f, base: "https://" + BnFHost}
}

// Name identifies this provider in stored provenance.
func (b *BnF) Name() string { return "bnf" }

// ByISBN looks one ISBN up.
//
// The bib.fuzzyISBN index is exactly as fuzzy as its name says: asked for an
// ISBN it does not have, it cheerfully returns unrelated books. Every record
// is therefore checked against the identifier that was asked for, or a
// scanned barcode would file a Dutch shipping history under a French novel.
func (b *BnF) ByISBN(ctx context.Context, isbn13 string) (Record, error) {
	b.pace()

	target := b.base + "/api/SRU?" + url.Values{
		"version":        {"1.2"},
		"operation":      {"searchRetrieve"},
		"query":          {`bib.fuzzyISBN all "` + isbn13 + `"`},
		"recordSchema":   {"dublincore"},
		"maximumRecords": {"5"},
	}.Encode()

	body, err := b.fetch.Get(ctx, target)
	if err != nil {
		return Record{}, fmt.Errorf("bnf: %w", err)
	}

	var answer bnfAnswer
	if err := xml.Unmarshal(body, &answer); err != nil {
		return Record{}, fmt.Errorf("bnf sent something unreadable: %w", err)
	}

	for _, record := range answer.Records {
		if !carriesISBN(record.Identifiers, isbn13) {
			continue
		}
		return record.toRecord(isbn13, b.Name()), nil
	}
	return Record{}, ErrNotFound
}

func (b *BnF) pace() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if wait := bnfPace - time.Since(b.lastCall); wait > 0 {
		time.Sleep(wait)
	}
	b.lastCall = time.Now()
}

// carriesISBN reports whether any identifier of the record is the ISBN that
// was asked for. Identifiers arrive as "ISBN 9782021563313" and sometimes
// hyphenated, so everything but the digits is dropped before comparing.
func carriesISBN(identifiers []string, want string) bool {
	for _, raw := range identifiers {
		if digitsOf(raw) == want {
			return true
		}
	}
	return false
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

type bnfAnswer struct {
	Records []bnfRecord `xml:"records>record>recordData>dc"`
}

type bnfRecord struct {
	Titles      []string `xml:"title"`
	Creators    []string `xml:"creator"`
	Publishers  []string `xml:"publisher"`
	Dates       []string `xml:"date"`
	Identifiers []string `xml:"identifier"`
	Formats     []string `xml:"format"`
	Languages   []string `xml:"language"`
}

func (r bnfRecord) toRecord(isbn13, provider string) Record {
	title, subtitle := splitTitle(first(r.Titles))

	out := Record{
		ISBN13:    isbn13,
		Title:     title,
		Subtitle:  subtitle,
		Publisher: cleanPublisher(first(r.Publishers)),
		Published: clean(first(r.Dates)),
		PageCount: pagesIn(first(r.Formats)),
		Provider:  provider,
	}
	for _, creator := range r.Creators {
		if name := cleanCreator(creator); name != "" {
			out.Authors = append(out.Authors, name)
		}
	}
	return out
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// splitTitle takes apart the one field the BnF packs three things into:
// "Le mystère de la maison aux trois ormes : roman / Valentin Musso".
// The author is already in dc:creator, so the tail is dropped rather than
// repeated.
func splitTitle(raw string) (title, subtitle string) {
	raw = clean(raw)
	if cut, _, found := strings.Cut(raw, " / "); found {
		raw = cut
	}
	if head, tail, found := strings.Cut(raw, " : "); found {
		return strings.TrimSpace(head), strings.TrimSpace(tail)
	}
	return raw, ""
}

// creatorRole matches the trailing role and life dates the BnF appends:
// "Musso, Valentin (1977-....). Auteur du texte".
var creatorRole = regexp.MustCompile(`\s*\([^)]*\)\s*\.?.*$`)

// cleanCreator turns a catalogue heading into the name a person would write.
func cleanCreator(raw string) string {
	name := creatorRole.ReplaceAllString(clean(raw), "")
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")

	// "Musso, Valentin" is how a catalogue files a name, not how anyone
	// writes it on a shelf.
	if family, given, found := strings.Cut(name, ", "); found {
		return strings.TrimSpace(given) + " " + strings.TrimSpace(family)
	}
	return name
}

// cleanPublisher drops the place of publication the BnF appends:
// "Éditions du Seuil (Paris)".
func cleanPublisher(raw string) string {
	publisher := clean(raw)
	if cut, _, found := strings.Cut(publisher, " ("); found {
		publisher = cut
	}
	return strings.TrimSpace(publisher)
}

// pagesCount matches the page count inside a physical description such as
// "1 vol. (364 p.) ; 22 cm".
var pagesCount = regexp.MustCompile(`\((\d+)\s*p\.?\)`)

func pagesIn(raw string) int {
	match := pagesCount.FindStringSubmatch(raw)
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}
