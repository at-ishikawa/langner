DROP INDEX IF EXISTS idx_cli_refresh_tokens_family_id;
ALTER TABLE cli_refresh_tokens DROP COLUMN IF EXISTS family_id;
