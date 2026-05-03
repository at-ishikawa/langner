-- The original etymology_origins unique key (notebook_id, origin, language)
-- can't represent multi-sense origins (e.g. "ana" = "up" in one session,
-- "negative" in another). Add session_title and key on it so each sense
-- gets its own row.

-- session_title is bounded to VARCHAR(100): real session names like
-- "Session 13" or "graphein (to write)" stay well under that. Keeping it
-- short is what lets the four-column unique index fit MySQL's 3072-byte
-- limit on utf8mb4 (255 + 100 + 255 + 100) * 4 = 2840 bytes.
ALTER TABLE etymology_origins
    ADD COLUMN session_title VARCHAR(100) NOT NULL DEFAULT ''
        COMMENT 'metadata.title of the etymology session that defines this sense'
        AFTER notebook_id;

-- Drop the old unique constraint and recreate it including session_title.
-- MySQL named the original UNIQUE(notebook_id, origin, language) constraint
-- after its leading column "notebook_id".
ALTER TABLE etymology_origins
    DROP INDEX notebook_id;

ALTER TABLE etymology_origins
    ADD UNIQUE KEY uniq_session_origin (notebook_id, session_title, origin, language);
