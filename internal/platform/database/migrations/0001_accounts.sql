-- Accounts and the library that owns everything else.
--
-- These four tables are the only ones that do not carry library_id: an account
-- exists before it is a member of anything. See docs/adr/0005-library-scoping.md.

CREATE TABLE app_user (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    password_hash TEXT,
    totp_secret   TEXT,
    is_admin      INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    locale        TEXT NOT NULL DEFAULT 'en',
    created_at    TEXT NOT NULL
) STRICT;

CREATE TABLE library (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE membership (
    user_id    TEXT NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES library(id)  ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'reader')),
    created_at TEXT NOT NULL,
    PRIMARY KEY (user_id, library_id)
) STRICT;

CREATE TABLE user_session (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    user_agent   TEXT,
    revoked_at   TEXT
) STRICT;

CREATE INDEX user_session_by_user ON user_session (user_id);
