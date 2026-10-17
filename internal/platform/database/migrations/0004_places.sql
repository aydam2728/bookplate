-- Where a copy physically sits.
--
-- The tree has no fixed levels: "House > Living room > Ikea bookcase >
-- Shelf 3" and "Garage > Box 7" must both be expressible, because a household
-- does not organise itself the way a cataloguer would.
--
-- path is a materialised path of the form '/<id>/<id>/<id>/'. The ancestors
-- of a place are exactly the places whose path is a prefix of its own, which
-- turns "walk up the tree" into one indexed comparison instead of recursion.

CREATE TABLE place (
    id                 TEXT PRIMARY KEY,
    library_id         TEXT NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    parent_id          TEXT REFERENCES place(id) ON DELETE RESTRICT,
    name               TEXT NOT NULL,
    kind               TEXT CHECK (kind IN
                         ('building','room','furniture','shelf','box','other')),
    path               TEXT NOT NULL,
    depth              INTEGER NOT NULL,
    position           INTEGER NOT NULL DEFAULT 0,
    subtree_copy_count INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL,
    deleted_at         TEXT
) STRICT;

-- A copy may sit nowhere in particular: "somewhere in the office" is a valid
-- answer, and so is "not filed yet".
ALTER TABLE copy ADD COLUMN place_id TEXT REFERENCES place(id) ON DELETE SET NULL;

CREATE INDEX place_by_parent  ON place (library_id, parent_id);
CREATE INDEX place_by_path    ON place (library_id, path);
CREATE INDEX copy_by_place    ON copy (library_id, place_id) WHERE deleted_at IS NULL;

-- The subtree counter, maintained in step with the copies.
-- See docs/adr/0008-three-denormalised-counters.md.

CREATE TRIGGER copy_ai_subtree AFTER INSERT ON copy
WHEN NEW.place_id IS NOT NULL AND NEW.deleted_at IS NULL
BEGIN
    UPDATE place
       SET subtree_copy_count = subtree_copy_count + 1
     WHERE library_id = NEW.library_id
       AND instr((SELECT path FROM place WHERE id = NEW.place_id), path) = 1;
END;

-- A move and a soft delete both arrive as an UPDATE: take one off the old
-- subtree, add one to the new.
CREATE TRIGGER copy_au_subtree AFTER UPDATE OF place_id, deleted_at ON copy
BEGIN
    UPDATE place
       SET subtree_copy_count = subtree_copy_count - 1
     WHERE OLD.place_id IS NOT NULL AND OLD.deleted_at IS NULL
       AND library_id = OLD.library_id
       AND instr((SELECT path FROM place WHERE id = OLD.place_id), path) = 1;
    UPDATE place
       SET subtree_copy_count = subtree_copy_count + 1
     WHERE NEW.place_id IS NOT NULL AND NEW.deleted_at IS NULL
       AND library_id = NEW.library_id
       AND instr((SELECT path FROM place WHERE id = NEW.place_id), path) = 1;
END;

CREATE TRIGGER copy_ad_subtree AFTER DELETE ON copy
WHEN OLD.place_id IS NOT NULL AND OLD.deleted_at IS NULL
BEGIN
    UPDATE place
       SET subtree_copy_count = subtree_copy_count - 1
     WHERE library_id = OLD.library_id
       AND instr((SELECT path FROM place WHERE id = OLD.place_id), path) = 1;
END;
