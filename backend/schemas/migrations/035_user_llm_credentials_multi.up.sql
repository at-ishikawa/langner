-- Allow a user to register a key for MORE THAN ONE provider so quiz grading can
-- fail over from their primary provider to another on a rate-limit / quota
-- error (PR #85, per-user). Was one row per user (user_id PRIMARY KEY); now one
-- row per (user_id, provider), with is_primary marking the provider the
-- resolver uses first (the other configured provider is the automatic
-- fallback).
ALTER TABLE user_llm_credentials DROP CONSTRAINT user_llm_credentials_pkey;
ALTER TABLE user_llm_credentials ADD PRIMARY KEY (user_id, provider);

ALTER TABLE user_llm_credentials ADD COLUMN is_primary BOOLEAN NOT NULL DEFAULT FALSE;

-- Existing single-credential users keep their one key as the primary.
UPDATE user_llm_credentials SET is_primary = TRUE;

-- At most one primary provider per user.
CREATE UNIQUE INDEX idx_user_llm_credentials_primary
    ON user_llm_credentials (user_id) WHERE is_primary;
