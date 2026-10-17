-- What each person thinks of a work, as opposed to what the household owns.
--
-- The rating hangs on the work, not the edition and not the copy: nobody
-- rates a binding. See docs/adr/0004-work-edition-copy.md.
--
-- One row per person per work, so a household of five has five opinions of
-- one book and they do not overwrite each other.

CREATE TABLE user_work (
    user_id     TEXT NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    work_id     TEXT NOT NULL REFERENCES work(id)     ON DELETE CASCADE,
    library_id  TEXT NOT NULL REFERENCES library(id)  ON DELETE CASCADE,
    -- Half stars, stored whole: 1 is half a star, 10 is five.
    rating      INTEGER CHECK (rating BETWEEN 1 AND 10),
    review      TEXT,
    visibility  TEXT NOT NULL DEFAULT 'private'
                  CHECK (visibility IN ('private', 'household')),
    status      TEXT CHECK (status IN ('to_read','reading','read','abandoned')),
    started_at  TEXT,
    finished_at TEXT,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, work_id)
) STRICT;

CREATE INDEX user_work_by_work ON user_work (library_id, work_id);

-- The household average is read on every row of every list, so it is counted
-- rather than recomputed. See docs/adr/0008-three-denormalised-counters.md.
ALTER TABLE work ADD COLUMN rating_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work ADD COLUMN rating_sum   INTEGER NOT NULL DEFAULT 0;

CREATE TRIGGER user_work_ai_rating AFTER INSERT ON user_work
WHEN NEW.rating IS NOT NULL
BEGIN
    UPDATE work SET rating_count = rating_count + 1,
                    rating_sum   = rating_sum + NEW.rating
     WHERE id = NEW.work_id;
END;

-- Setting, changing and clearing a rating all arrive as an UPDATE.
CREATE TRIGGER user_work_au_rating AFTER UPDATE OF rating ON user_work
BEGIN
    UPDATE work SET rating_count = rating_count - 1,
                    rating_sum   = rating_sum - OLD.rating
     WHERE OLD.rating IS NOT NULL AND id = OLD.work_id;
    UPDATE work SET rating_count = rating_count + 1,
                    rating_sum   = rating_sum + NEW.rating
     WHERE NEW.rating IS NOT NULL AND id = NEW.work_id;
END;

CREATE TRIGGER user_work_ad_rating AFTER DELETE ON user_work
WHEN OLD.rating IS NOT NULL
BEGIN
    UPDATE work SET rating_count = rating_count - 1,
                    rating_sum   = rating_sum - OLD.rating
     WHERE id = OLD.work_id;
END;
