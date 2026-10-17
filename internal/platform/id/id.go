// Package id generates ULIDs.
//
// A ULID is a 128-bit identifier: 48 bits of millisecond timestamp followed by
// 80 bits of randomness, rendered as 26 Crockford base32 characters. It sorts
// chronologically as a string, which keeps index locality in SQLite, and it
// reveals nothing about how many rows exist, unlike an auto-increment column
// in a URL.
//
// Implemented here rather than pulled in: the specification is short, and a
// dependency is a decision this project does not make lightly.
package id

import (
	"crypto/rand"
	"fmt"
	"time"
)

// crockford is base32 without I, L, O and U, so that a misread identifier is
// far less likely to be a valid one.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Length is the number of characters in a rendered ULID.
const Length = 26

// New returns a new ULID for the current time.
func New() (string, error) {
	return at(time.Now())
}

func at(t time.Time) (string, error) {
	var raw [16]byte

	ms := uint64(t.UTC().UnixMilli())
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)

	if _, err := rand.Read(raw[6:]); err != nil {
		return "", fmt.Errorf("generating an identifier: %w", err)
	}

	// 16 bytes is 128 bits, which is 25.6 base32 characters, so the first
	// character carries only the top 2 bits of the timestamp.
	out := make([]byte, Length)
	var bits uint16
	var count uint
	pos := Length
	for i := 15; i >= 0; i-- {
		bits |= uint16(raw[i]) << count
		count += 8
		for count >= 5 {
			pos--
			out[pos] = crockford[bits&0x1f]
			bits >>= 5
			count -= 5
		}
	}
	for pos > 0 {
		pos--
		out[pos] = crockford[bits&0x1f]
		bits >>= 5
	}
	return string(out), nil
}
