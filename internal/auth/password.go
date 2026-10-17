// Package auth handles accounts, passwords and sessions.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters, following the OWASP recommendation. Memory dominates
// the cost, and 19 MiB per attempt stays affordable on a Raspberry Pi while
// being expensive enough to make offline cracking slow.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// MinPasswordLength follows the modern guidance: length, no composition
// rules. Rules such as "one capital and one digit" push people towards
// Password1! and buy nothing.
const MinPasswordLength = 10

// ErrPasswordTooShort reports a password below MinPasswordLength.
var ErrPasswordTooShort = errors.New("password is too short")

// ErrMalformedHash reports a stored hash that cannot be parsed. It means the
// stored value was corrupted or written by something else.
var ErrMalformedHash = errors.New("malformed password hash")

// HashPassword returns an encoded Argon2id hash in the standard PHC string
// format, so the parameters travel with the hash and can be raised later
// without invalidating existing passwords.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "", fmt.Errorf("%w: need at least %d characters",
			ErrPasswordTooShort, MinPasswordLength)
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating a salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the encoded hash.
//
// It compares in constant time: a comparison that returns early leaks how
// much of the hash matched, one byte at a time.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("%w: not an argon2id hash", ErrMalformedHash)
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("%w: unreadable version", ErrMalformedHash)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("%w: unsupported version %d", ErrMalformedHash, version)
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("%w: unreadable parameters", ErrMalformedHash)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("%w: unreadable salt", ErrMalformedHash)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("%w: unreadable key", ErrMalformedHash)
	}

	// argon2.IDKey panics on a zero key length or a zero cost parameter, and
	// a corrupted row in the database must not be able to crash the server on
	// the request path.
	if len(salt) == 0 || len(want) == 0 {
		return false, fmt.Errorf("%w: empty salt or key", ErrMalformedHash)
	}
	if memory == 0 || time == 0 || threads == 0 {
		return false, fmt.Errorf("%w: zero cost parameter", ErrMalformedHash)
	}

	// The parameters come from the stored hash, not from the constants above,
	// so raising the cost later leaves existing passwords verifiable.
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))

	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
