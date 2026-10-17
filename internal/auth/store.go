package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aydam2728/bookplate/internal/platform/id"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// ErrNotFound reports an account or session that does not exist. Login
// reports it for a wrong password too, so that the response cannot be used to
// discover which addresses have accounts.
var ErrNotFound = errors.New("not found")

// ErrAlreadySetUp reports a second attempt at first-run setup.
var ErrAlreadySetUp = errors.New("this instance already has an account")

// SessionTTL is how long a session stays valid without being refreshed.
const SessionTTL = 30 * 24 * time.Hour

// Store reads and writes accounts, memberships and sessions.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store over db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// NeedsSetup reports whether this instance has no account yet. The first run
// creates the administrator; there is no default password to leak.
func (s *Store) NeedsSetup(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM app_user`).Scan(&count); err != nil {
		return false, fmt.Errorf("counting accounts: %w", err)
	}
	return count == 0, nil
}

// Setup creates the first account and its library in one transaction. It
// refuses to run twice.
func (s *Store) Setup(ctx context.Context, email, displayName, password, libraryName string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	userID, err := id.New()
	if err != nil {
		return err
	}
	libraryID, err := id.New()
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM app_user`).Scan(&count); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if count > 0 {
		return ErrAlreadySetUp
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO app_user (id, email, display_name, password_hash, is_admin, locale, created_at)
		 VALUES (?, ?, ?, ?, 1, 'en', ?)`,
		userID, normaliseEmail(email), strings.TrimSpace(displayName), hash, now)
	if err != nil {
		return fmt.Errorf("creating the first account: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO library (id, name, created_at) VALUES (?, ?, ?)`,
		libraryID, strings.TrimSpace(libraryName), now)
	if err != nil {
		return fmt.Errorf("creating the first library: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO membership (user_id, library_id, role, created_at) VALUES (?, ?, 'owner', ?)`,
		userID, libraryID, now)
	if err != nil {
		return fmt.Errorf("creating the first membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	return nil
}

// Login verifies a password and opens a session, returning the token to put
// in the cookie. The token is returned once and stored only as a hash.
func (s *Store) Login(ctx context.Context, email, password, userAgent string) (string, error) {
	var userID, hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, coalesce(password_hash, '') FROM app_user WHERE email = ?`,
		normaliseEmail(email)).Scan(&userID, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Spend time anyway: answering instantly for unknown addresses tells
		// an attacker which ones exist.
		_, _ = VerifyPassword(decoyHash, password)
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	if hash == "" {
		return "", ErrNotFound
	}

	ok, err := VerifyPassword(hash, password)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	if !ok {
		return "", ErrNotFound
	}

	return s.openSession(ctx, userID, userAgent)
}

func (s *Store) openSession(ctx context.Context, userID, userAgent string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("opening a session: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_session (id, user_id, created_at, expires_at, last_seen_at, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		hashToken(token), userID,
		now.Format(time.RFC3339),
		now.Add(SessionTTL).Format(time.RFC3339),
		now.Format(time.RFC3339),
		userAgent)
	if err != nil {
		return "", fmt.Errorf("opening a session: %w", err)
	}
	return token, nil
}

// Logout revokes a session. Logging out of an unknown session is not an
// error: the outcome the caller wanted is already true.
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_session SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), hashToken(token))
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// Caller resolves a session token into the scope its holder acts in.
//
// The library is the one the account is a member of. Until an account can
// belong to several, the most recently joined wins.
func (s *Store) Caller(ctx context.Context, token string) (scope.Caller, error) {
	var c scope.Caller
	var expires string
	var isAdmin int
	err := s.db.QueryRowContext(ctx,
		`SELECT s.expires_at, u.id, u.is_admin, m.library_id, m.role
		   FROM user_session s
		   JOIN app_user   u ON u.id = s.user_id
		   JOIN membership m ON m.user_id = u.id
		  WHERE s.id = ? AND s.revoked_at IS NULL
		  ORDER BY m.created_at DESC
		  LIMIT 1`,
		hashToken(token)).Scan(&expires, &c.UserID, &isAdmin, &c.LibraryID, &c.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return scope.Caller{}, ErrNotFound
	}
	if err != nil {
		return scope.Caller{}, fmt.Errorf("resolving the session: %w", err)
	}

	at, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return scope.Caller{}, fmt.Errorf("unreadable session expiry: %w", err)
	}
	if time.Now().UTC().After(at) {
		return scope.Caller{}, ErrNotFound
	}

	c.IsAdmin = isAdmin == 1
	if !c.Valid() {
		return scope.Caller{}, ErrNotFound
	}
	return c, nil
}

// hashToken stores sessions by digest. Someone who reads the database still
// cannot replay a live session.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// decoyHash is a real argon2id hash of a value nobody knows, used to keep the
// cost of a login attempt the same whether or not the address exists.
const decoyHash = "$argon2id$v=19$m=19456,t=2,p=1$" +
	"ZGVjb3lzYWx0ZGVjb3lzYQ$" +
	"Ry6rJ3Mb8aUqWvS1cMbZ5nFZ8xH0lP2kJqR3tVwXyZ0"
