-- The bibliographic core: a work is what was read, an edition is the
-- editorial object with an ISBN, a copy is the object on a shelf.
-- See docs/adr/0004-work-edition-copy.md.
--
-- Every table here carries library_id, even where a join could derive it, so
-- that the isolation clause is uniform and indexable.

CREATE TABLE work (
    id             TEXT PRIMARY KEY,
    library_id     TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    subtitle       TEXT,
    series_name    TEXT,
    series_index   REAL,
    summary        TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
) STRICT;

CREATE TABLE author (
    id         TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    sort_name  TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (library_id, sort_name)
) STRICT;

CREATE TABLE work_author (
    work_id    TEXT NOT NULL REFERENCES work(id)    ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES author(id)  ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'author'
                 CHECK (role IN ('author', 'translator', 'illustrator', 'editor')),
    position   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (work_id, author_id, role)
) STRICT;

CREATE TABLE edition (
    id             TEXT PRIMARY KEY,
    library_id     TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    work_id        TEXT NOT NULL REFERENCES work(id)    ON DELETE CASCADE,
    publisher      TEXT,
    published_date TEXT,
    language       TEXT,
    page_count     INTEGER,
    format         TEXT CHECK (format IN
                     ('paperback','hardcover','pocket','comic','audio','ebook','other')),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
) STRICT;

CREATE TABLE edition_identifier (
    edition_id TEXT NOT NULL REFERENCES edition(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    scheme     TEXT NOT NULL CHECK (scheme IN
                 ('isbn13','isbn10','ean','openlibrary','bnf','google')),
    value      TEXT NOT NULL,
    PRIMARY KEY (edition_id, scheme, value)
) STRICT;

CREATE TABLE copy (
    id          TEXT PRIMARY KEY,
    library_id  TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    edition_id  TEXT NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
    status      TEXT NOT NULL DEFAULT 'owned'
                  CHECK (status IN ('owned','wanted','lent','lost')),
    condition   TEXT CHECK (condition IN ('new','good','worn','damaged')),
    acquired_at TEXT,
    price_cents INTEGER,
    currency    TEXT,
    notes       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT
) STRICT;

-- Every foreign key gets an index, and every index starts with library_id,
-- because no query runs without filtering on it.
CREATE INDEX work_by_library        ON work (library_id, title);
CREATE INDEX edition_by_work        ON edition (library_id, work_id);
CREATE INDEX copy_by_edition        ON copy (library_id, edition_id) WHERE deleted_at IS NULL;
CREATE INDEX work_author_by_author  ON work_author (author_id);
CREATE INDEX work_author_by_work    ON work_author (library_id, work_id);

-- An ISBN-13 identifies one edition within a library. This index is what makes
-- "do we already own it?" a single lookup.
CREATE UNIQUE INDEX edition_isbn13_unique
    ON edition_identifier (library_id, value)
    WHERE scheme = 'isbn13';
