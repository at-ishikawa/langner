-- Per-login refresh-token families. A "family" is the rotation chain descended
-- from ONE sign-in (a web login, or a single CLI device). Reuse of an
-- already-rotated token revokes only its family, so a compromise (or replay) on
-- one client — web or a CLI device — no longer logs the user's OTHER clients
-- out. Previously the compromise response revoked every token for the user_id,
-- coupling independent sessions together.
ALTER TABLE cli_refresh_tokens ADD COLUMN family_id TEXT NOT NULL DEFAULT '';
-- Existing rows predate families; give each its own so none are grouped.
UPDATE cli_refresh_tokens SET family_id = id::text WHERE family_id = '';
ALTER TABLE cli_refresh_tokens ALTER COLUMN family_id DROP DEFAULT;
CREATE INDEX idx_cli_refresh_tokens_family_id ON cli_refresh_tokens (family_id);
