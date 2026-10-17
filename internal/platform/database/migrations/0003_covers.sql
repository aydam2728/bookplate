-- Cached cover images.
--
-- The file on disk is named after the SHA-256 of its re-encoded bytes, never
-- after anything that came from outside: a name taken from a URL or a header
-- is a path traversal waiting to happen.
--
-- source_url and attribution are kept because covers are copyrighted works.
-- The project redistributes none of them; an operator who needs to know where
-- an image came from can see it.

CREATE TABLE cover_asset (
    id          TEXT PRIMARY KEY,
    library_id  TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    sha256      TEXT NOT NULL,
    mime_type   TEXT NOT NULL,
    width       INTEGER NOT NULL,
    height      INTEGER NOT NULL,
    byte_size   INTEGER NOT NULL,
    source_url  TEXT,
    attribution TEXT,
    fetched_at  TEXT NOT NULL,
    UNIQUE (library_id, sha256)
) STRICT;

ALTER TABLE edition ADD COLUMN cover_asset_id TEXT REFERENCES cover_asset(id) ON DELETE SET NULL;

CREATE INDEX cover_asset_by_library ON cover_asset (library_id);
