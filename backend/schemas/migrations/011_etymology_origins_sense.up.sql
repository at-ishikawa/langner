ALTER TABLE etymology_origins
    ADD COLUMN sense VARCHAR(50) NOT NULL DEFAULT '';

ALTER TABLE etymology_origins
    DROP CONSTRAINT uniq_session_origin;

ALTER TABLE etymology_origins
    ADD CONSTRAINT uniq_session_origin UNIQUE (notebook_id, session_title, origin, language, sense);
