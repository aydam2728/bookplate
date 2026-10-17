package auth_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/auth"
	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

func newStore(t *testing.T) (*auth.Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return auth.NewStore(db), db
}

// setUp runs first-run setup and returns the credentials it used.
func setUp(t *testing.T, s *auth.Store) (email, password string) {
	t.Helper()
	email, password = "owner@example.org", "correct horse battery staple"
	if err := s.Setup(t.Context(), email, "Owner", password, "Home"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return email, password
}

func TestNeedsSetup_IsTrueOnlyBeforeTheFirstAccount(t *testing.T) {
	s, _ := newStore(t)

	needs, err := s.NeedsSetup(t.Context())
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if !needs {
		t.Fatal("a fresh instance did not ask to be set up")
	}

	setUp(t, s)

	needs, err = s.NeedsSetup(t.Context())
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if needs {
		t.Error("the instance still asks to be set up after the first account")
	}
}

func TestSetup_RefusesToRunTwice(t *testing.T) {
	s, _ := newStore(t)
	setUp(t, s)

	err := s.Setup(t.Context(), "someone@example.org", "Someone", "another long password", "Theirs")
	if !errors.Is(err, auth.ErrAlreadySetUp) {
		t.Fatalf("a second setup was accepted, err = %v: anyone reaching the "+
			"instance could have taken it over", err)
	}
}

func TestSetup_CreatesAnOwnerOfItsLibrary(t *testing.T) {
	s, db := newStore(t)
	email, password := setUp(t, s)

	token, err := s.Login(t.Context(), email, password, "test")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	caller, err := s.Caller(t.Context(), token)
	if err != nil {
		t.Fatalf("Caller: %v", err)
	}
	if caller.Role != scope.RoleOwner {
		t.Errorf("role = %q, want %q", caller.Role, scope.RoleOwner)
	}
	if !caller.IsAdmin {
		t.Error("the first account is not an instance administrator")
	}
	if caller.LibraryID == "" {
		t.Error("the first account has no library")
	}

	var stored string
	err = db.QueryRowContext(t.Context(), `SELECT password_hash FROM app_user`).Scan(&stored)
	if err != nil {
		t.Fatalf("reading the stored hash: %v", err)
	}
	if stored == password {
		t.Fatal("the password is stored in clear")
	}
}

func TestLogin_WithTheWrongPassword_IsIndistinguishableFromAnUnknownAddress(t *testing.T) {
	s, _ := newStore(t)
	email, _ := setUp(t, s)

	_, wrongPassword := s.Login(t.Context(), email, "not the password at all", "test")
	_, unknownAddress := s.Login(t.Context(), "nobody@example.org", "not the password at all", "test")

	if !errors.Is(wrongPassword, auth.ErrNotFound) {
		t.Errorf("wrong password gave %v, want ErrNotFound", wrongPassword)
	}
	if !errors.Is(unknownAddress, auth.ErrNotFound) {
		t.Errorf("unknown address gave %v, want ErrNotFound", unknownAddress)
	}
	if wrongPassword.Error() != unknownAddress.Error() {
		t.Errorf("the two answers differ (%q vs %q), which tells an attacker "+
			"which addresses have accounts", wrongPassword, unknownAddress)
	}
}

func TestLogin_IsCaseInsensitiveOnTheAddress(t *testing.T) {
	s, _ := newStore(t)
	email, password := setUp(t, s)

	if _, err := s.Login(t.Context(), "  OWNER@Example.ORG  ", password, "test"); err != nil {
		t.Errorf("login refused a differently cased address: %v", err)
		_ = email
	}
}

func TestCaller_WhenTheTokenIsUnknown_Refuses(t *testing.T) {
	s, _ := newStore(t)
	setUp(t, s)

	if _, err := s.Caller(t.Context(), "not-a-real-token"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("an invented token resolved, err = %v", err)
	}
}

func TestLogout_RevokesTheSession(t *testing.T) {
	s, _ := newStore(t)
	email, password := setUp(t, s)

	token, err := s.Login(t.Context(), email, password, "test")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := s.Caller(t.Context(), token); err != nil {
		t.Fatalf("the session did not work before logout: %v", err)
	}

	if err := s.Logout(t.Context(), token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.Caller(t.Context(), token); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("the session still works after logout, err = %v", err)
	}
}

func TestLogin_StoresTheSessionByDigestOnly(t *testing.T) {
	s, db := newStore(t)
	email, password := setUp(t, s)

	token, err := s.Login(t.Context(), email, password, "test")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	var stored string
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM user_session`).Scan(&stored); err != nil {
		t.Fatalf("reading the session row: %v", err)
	}
	if stored == token {
		t.Error("the session token is stored verbatim, so anyone who can read " +
			"the database can replay a live session")
	}
}
