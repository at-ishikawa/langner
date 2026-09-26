ALTER TABLE notebooks DROP COLUMN IF EXISTS content_hash;
ALTER TABLE notebooks DROP COLUMN IF EXISTS display_name;
ALTER TABLE notebooks DROP COLUMN IF EXISTS kind;
ALTER TABLE notebooks DROP COLUMN IF EXISTS source;

DROP TRIGGER IF EXISTS notebook_files_set_updated_at ON notebook_files;
DROP TABLE IF EXISTS notebook_files;
