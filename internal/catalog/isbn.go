package catalog

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotABook reports a barcode that is valid but does not identify a book,
// such as a periodical. It deserves a message of its own rather than a
// generic failure, because scanning a magazine is an easy mistake to make.
var ErrNotABook = errors.New("this barcode is not a book")

// ErrBadISBN reports an identifier that is not a usable ISBN.
var ErrBadISBN = errors.New("not a valid ISBN")

// NormaliseISBN turns anything a scanner or a keyboard produces into an
// ISBN-13, which is the only form stored.
//
// It accepts ISBN-10, ISBN-13 and EAN-13, with or without hyphens and spaces,
// and tolerates the five digit price add-on some scanners append. The check
// digit is verified before any network call is made: a mistyped identifier
// should fail locally and instantly.
func NormaliseISBN(input string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r == 'X' || r == 'x':
			return 'X'
		default:
			return -1
		}
	}, input)

	// Some scanners append a five digit price add-on to the barcode.
	if len(digits) == 18 || len(digits) == 15 {
		digits = digits[:len(digits)-5]
	}

	switch len(digits) {
	case 10:
		return fromISBN10(digits)
	case 13:
		return fromISBN13(digits)
	default:
		return "", fmt.Errorf("%w: %q has %d usable characters, expected 10 or 13",
			ErrBadISBN, input, len(digits))
	}
}

func fromISBN13(digits string) (string, error) {
	if strings.ContainsRune(digits, 'X') {
		return "", fmt.Errorf("%w: X is only valid as an ISBN-10 check digit", ErrBadISBN)
	}
	switch digits[:3] {
	case "978", "979":
	case "977":
		return "", fmt.Errorf("%w: 977 is a periodical (ISSN), not a book", ErrNotABook)
	default:
		return "", fmt.Errorf("%w: %q is not a Bookland prefix", ErrNotABook, digits[:3])
	}
	if checkDigit13(digits[:12]) != digits[12] {
		return "", fmt.Errorf("%w: the check digit of %q does not match", ErrBadISBN, digits)
	}
	return digits, nil
}

func fromISBN10(digits string) (string, error) {
	if strings.ContainsRune(digits[:9], 'X') {
		return "", fmt.Errorf("%w: X is only valid as the final character", ErrBadISBN)
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(digits[i]-'0') * (10 - i)
	}
	last := digits[9]
	switch {
	case last == 'X':
		sum += 10
	default:
		sum += int(last - '0')
	}
	if sum%11 != 0 {
		return "", fmt.Errorf("%w: the check digit of %q does not match", ErrBadISBN, digits)
	}

	body := "978" + digits[:9]
	return body + string(checkDigit13(body)), nil
}

// checkDigit13 returns the final character of an ISBN-13 given its first
// twelve digits, using the alternating 1 and 3 weighting of EAN-13.
func checkDigit13(twelve string) byte {
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(twelve[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}
