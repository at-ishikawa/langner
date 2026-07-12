-- relearn_clears records that a word was recovered in a Relearn Quiz session.
-- It is deliberately NOT a learning log: it has no quiz_type, status, quality,
-- interval, or easiness factor, and is never read by SM-2 or analytics. Its
-- only reader is the Relearn pool builder, which excludes a word when its
-- clear time is newer than the word's most-recent in-window wrong log.
--
-- The key is a text clear_key ("<notebook>\x00<lowercased expression>"), not a
-- note_id FK: the Relearn pool is built from the YAML learning histories (the
-- source of truth, and the only place etymology-origin results are stored), so
-- the pool includes etymology origins — which have no row in `notes` and thus
-- no note_id. A text key is the only representation that covers both vocab and
-- etymology words. One row per key; a new clear upserts cleared_at.
CREATE TABLE relearn_clears (
    clear_key VARCHAR(512) NOT NULL,
    cleared_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (clear_key)
);
CREATE INDEX idx_relearn_clears_cleared_at ON relearn_clears (cleared_at);
