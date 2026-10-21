package catalog

import "strings"

// fold reduces a string to what two people typing the same title would both
// produce: lower case, no diacritics, straight punctuation.
//
// SQLite's lower() and like are ASCII-only without ICU, so "etagere" would
// never find "Étagère" in SQL. This catalogue mostly holds French books and
// cannot be searched that way, so the rule lives here, in one tested place,
// and the matching happens in Go.
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r >= 0x0300 && r <= 0x036f:
			// A combining mark: the accent of a decomposed "é", already
			// handled by the letter in front of it.
		default:
			b.WriteString(foldRune(r))
		}
	}
	return b.String()
}

// folded maps the letters this household will actually type. The table is
// deliberately not exhaustive: an unknown rune is kept as it is, so a
// Cyrillic or Japanese title still matches itself.
var folded = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'ç': "c", 'ć': "c", 'č': "c",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ī': "i", 'į': "i",
	'ñ': "n", 'ń': "n", 'ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u",
	'ý': "y", 'ÿ': "y",
	'ś': "s", 'š': "s", 'ș': "s",
	'ť': "t", 'ț': "t",
	'ź': "z", 'ż': "z", 'ž': "z",
	'ĺ': "l", 'ľ': "l", 'ł': "l",
	'ŕ': "r", 'ř': "r",
	'ď': "d", 'đ': "d",
	'ğ': "g",
	'æ': "ae", 'œ': "oe", 'ß': "ss",
	// Punctuation a word processor substitutes behind a person's back.
	'’': "'", '‘': "'", '“': `"`, '”': `"`, '–': "-", '—': "-", ' ': " ",
}

func foldRune(r rune) string {
	if s, ok := folded[r]; ok {
		return s
	}
	return string(r)
}

// digits keeps only the digits, which is how a barcode typed with hyphens
// comes to match the ISBN stored without them.
func digits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}

// matches reports whether a book answers to every word typed.
//
// Every word has to appear somewhere, in any order: "tolkien hobbit" and
// "hobbit tolkien" find the same book. A word made of digits is also tried
// against the ISBN, so a barcode finds the copy whether or not the person
// typed the hyphens.
//
// The summary is searched too. It is the one field a household writes itself,
// and "the one about the badger" is how people remember books.
func matches(b Book, words []string) bool {
	if len(words) == 0 {
		return true
	}
	haystack := fold(strings.Join([]string{b.Title, b.Authors, b.Publisher,
		b.Published, b.PlaceName, b.Summary, b.Notes}, "\n"))
	isbn := digits(b.ISBN13)

	for _, word := range words {
		if strings.Contains(haystack, word) {
			continue
		}
		if d := digits(word); d != "" && strings.Contains(isbn, d) {
			continue
		}
		return false
	}
	return true
}

// words folds a typed query into the words to look for.
func words(text string) []string {
	return strings.Fields(fold(text))
}
