DROP TRIGGER IF EXISTS notebooks_set_updated_at ON notebooks;
DROP INDEX IF EXISTS idx_notebooks_owner_user_id;
DROP TABLE IF EXISTS notebooks;
