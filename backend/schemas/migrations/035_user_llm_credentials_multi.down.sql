-- Revert to one credential per user: keep each user's primary (or an arbitrary
-- row if none is flagged), drop the rest, then restore the user_id PRIMARY KEY.
DELETE FROM user_llm_credentials a
    USING user_llm_credentials b
    WHERE a.user_id = b.user_id
      AND a.provider <> b.provider
      AND b.is_primary
      AND NOT a.is_primary;
-- For any user with no primary flagged, keep the lexically-first provider.
DELETE FROM user_llm_credentials a
    USING user_llm_credentials b
    WHERE a.user_id = b.user_id
      AND a.provider > b.provider
      AND NOT EXISTS (
          SELECT 1 FROM user_llm_credentials p
          WHERE p.user_id = a.user_id AND p.is_primary
      );

DROP INDEX IF EXISTS idx_user_llm_credentials_primary;
ALTER TABLE user_llm_credentials DROP COLUMN is_primary;
ALTER TABLE user_llm_credentials DROP CONSTRAINT user_llm_credentials_pkey;
ALTER TABLE user_llm_credentials ADD PRIMARY KEY (user_id);
