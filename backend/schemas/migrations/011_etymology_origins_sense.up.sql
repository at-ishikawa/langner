-- Same-session multi-sense origins (e.g. "pathos" = "feeling" AND "pathos" =
-- "disease, suffering" both in Session 9 of a Greek roots notebook) collide
-- under the migration 008 unique key (notebook_id, session_title, origin,
-- language) — the second declaration is dropped at import. Add a `sense`
-- discriminator column so each declared sense gets its own row, with
-- definitions' origin_parts pinning the right sense via OriginPartRef.Sense.
--
-- sense is bounded to VARCHAR(50): a short symbolic token like "feeling" or
-- "disease". Default '' keeps the 99% of single-sense origins exactly where
-- they were under migration 008 — they upgrade with no YAML change.

ALTER TABLE etymology_origins
    ADD COLUMN sense VARCHAR(50) NOT NULL DEFAULT ''
        COMMENT 'short symbolic disambiguator for same-session multi-sense origins'
        AFTER session_title;

-- Drop the migration 008 unique constraint and recreate it including sense.
ALTER TABLE etymology_origins
    DROP INDEX uniq_session_origin;

ALTER TABLE etymology_origins
    ADD UNIQUE KEY uniq_session_origin (notebook_id, session_title, origin, language, sense);
