-- Notebook content lives in the content_* tables (migration 038), and
-- `langner-admin migrate schema` rebuilt every notebook's rows from its stored
-- notebook_files. The files were kept as a backup; drop them now.
--
-- Refuse while a stored notebook still needs those files:
--   * it has no content rows (never converted), or
--   * its stored files are its current bundle (they hash to content_hash, the
--     same hash as notebook.HashBundle) but its rows were not built from them
--     (content_built_hash differs, migration 039).
-- Run the previous release's `langner-admin migrate schema` first; it rebuilds
-- those rows from the files.
DO $$
BEGIN
    IF to_regclass('notebook_files') IS NULL THEN
        RETURN;
    END IF;
    IF EXISTS (
        WITH files AS (
            SELECT notebook_id,
                   encode(sha256(string_agg(
                       convert_to(family || '/' || path, 'UTF8') || '\x00'::bytea
                       || convert_to(encode(sha256(content), 'hex'), 'UTF8') || '\x00'::bytea,
                       ''::bytea ORDER BY family || '/' || path COLLATE "C")), 'hex') AS files_hash
            FROM notebook_files GROUP BY notebook_id
        )
        SELECT 1 FROM files f LEFT JOIN notebooks n ON n.notebook_id = f.notebook_id
        WHERE NOT EXISTS (SELECT 1 FROM content_indexes i WHERE i.notebook_id = f.notebook_id)
           OR (f.files_hash = n.content_hash AND n.content_built_hash IS DISTINCT FROM n.content_hash)
    ) THEN
        RAISE EXCEPTION 'notebook_files holds notebooks whose content rows were never built from them; run the previous release''s "langner-admin migrate schema" first';
    END IF;
END $$;

DROP TABLE IF EXISTS notebook_files;
