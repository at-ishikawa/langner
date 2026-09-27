-- Revert the family dimension. Note: if composite notebooks were imported (the
-- same (notebook_id, path) under multiple families), restoring the narrower
-- UNIQUE (notebook_id, path) will fail on the duplicate rows — drop those blobs
-- first. This is expected for a down migration on a catalog that used the
-- feature.
ALTER TABLE notebook_files DROP CONSTRAINT IF EXISTS notebook_files_notebook_id_family_path_key;
ALTER TABLE notebook_files ADD  CONSTRAINT notebook_files_notebook_id_path_key UNIQUE (notebook_id, path);
ALTER TABLE notebook_files DROP COLUMN IF EXISTS family;
