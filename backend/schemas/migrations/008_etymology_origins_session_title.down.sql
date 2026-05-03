ALTER TABLE etymology_origins
    DROP INDEX uniq_session_origin;

ALTER TABLE etymology_origins
    ADD UNIQUE KEY notebook_id (notebook_id, origin, language);

ALTER TABLE etymology_origins
    DROP COLUMN session_title;
