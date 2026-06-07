-- Add concept_key columns to support definitions-side word concepts grouping.
-- Notes carry the head expression of the concept they belong to; learning
-- logs cache the same so "logs for a concept" is a single-index lookup
-- without joining notes. Both default to '' so existing rows survive
-- cleanly until the per-concept log merge runs (migration is destructive
-- and ships as a separate `langner migrate merge-concepts` CLI command).

ALTER TABLE notes
    ADD COLUMN concept_key VARCHAR(255) NOT NULL DEFAULT ''
        COMMENT 'Head expression of the definitions concept this note belongs to, or "" if none'
        AFTER dictionary_number;

ALTER TABLE notes
    ADD INDEX idx_notes_concept_key (concept_key);

ALTER TABLE learning_logs
    ADD COLUMN concept_key VARCHAR(255) NOT NULL DEFAULT ''
        COMMENT 'Head expression of the definitions concept this log belongs to (denormalised cache of notes.concept_key)'
        AFTER interval_days;

ALTER TABLE learning_logs
    ADD INDEX idx_learning_logs_concept_key (concept_key);
