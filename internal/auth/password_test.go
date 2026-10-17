package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/auth"
)

const good = "correct horse battery staple"

func TestHashPassword_ProducesAnArgon2idPHCString(t *testing.T) {
	hash, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash does not announce its algorithm: %q", hash)
	}
	if n := len(strings.Split(hash, "$")); n != 6 {
		t.Errorf("hash has %d fields, want the 6 of a PHC string: %q", n, hash)
	}
	if strings.Contains(hash, good) {
		t.Error("the hash contains the password")
	}
}

func TestHashPassword_SaltsEveryHash(t *testing.T) {
	first, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if first == second {
		t.Error("the same password hashed twice gave the same value, so the salt " +
			"is not random and one leaked hash would unmask every reuse")
	}
}

func TestHashPassword_RefusesShortPasswords(t *testing.T) {
	_, err := auth.HashPassword("short")
	if !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Errorf("a five character password was accepted, err = %v", err)
	}
}

func TestVerifyPassword_AcceptsTheRightPassword(t *testing.T) {
	hash, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := auth.VerifyPassword(hash, good)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("the correct password was rejected")
	}
}

func TestVerifyPassword_RejectsTheWrongPassword(t *testing.T) {
	hash, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := auth.VerifyPassword(hash, good+"x")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Error("a wrong password was accepted")
	}
}

func TestVerifyPassword_WhenTheHashIsMalformed_ReportsIt(t *testing.T) {
	for _, bad := range []string{
		"",
		"plaintext",
		"$bcrypt$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=99$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$nonsense$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=1,t=1,p=1$!!!$aGFzaA",
	} {
		t.Run(bad, func(t *testing.T) {
			ok, err := auth.VerifyPassword(bad, good)
			if ok {
				t.Error("a malformed hash verified successfully")
			}
			if !errors.Is(err, auth.ErrMalformedHash) {
				t.Errorf("err = %v, want ErrMalformedHash", err)
			}
		})
	}
}

func TestVerifyPassword_WhenTheKeyIsEmpty_DoesNotCrash(t *testing.T) {
	// A corrupted row must produce an error, never a panic: this runs on the
	// request path of a login attempt.
	ok, err := auth.VerifyPassword("$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHQ$", good)
	if ok {
		t.Error("a hash with no key verified successfully")
	}
	if !errors.Is(err, auth.ErrMalformedHash) {
		t.Errorf("err = %v, want ErrMalformedHash", err)
	}
}

func TestVerifyPassword_WhenCostIsZero_DoesNotCrash(t *testing.T) {
	ok, err := auth.VerifyPassword("$argon2id$v=19$m=0,t=0,p=0$c29tZXNhbHQ$aGFzaA", good)
	if ok {
		t.Error("a hash with zero cost verified successfully")
	}
	if !errors.Is(err, auth.ErrMalformedHash) {
		t.Errorf("err = %v, want ErrMalformedHash", err)
	}
}

func TestHashPassword_RecordsItsParameters(t *testing.T) {
	// The parameters are read back from the stored hash when verifying, so
	// raising the cost later leaves existing passwords usable.
	hash, err := auth.HashPassword(good)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.Contains(hash, "m=19456,t=2,p=1") {
		t.Errorf("the cost parameters are not recorded in the hash: %q", hash)
	}
}
