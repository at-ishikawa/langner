ALTER TABLE etymology_origins
    DROP CONSTRAINT IF EXISTS uniq_session_origin;

ALTER TABLE etymology_origins
    DROP COLUMN IF EXISTS sense;

ALTER TABLE etymology_origins
    ADD CONSTRAINT uniq_session_origin UNIQUE (notebook_id, session_title, origin, language);
