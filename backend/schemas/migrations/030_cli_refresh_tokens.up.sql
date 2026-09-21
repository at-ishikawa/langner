-- Hashed CLI refresh tokens. The plaintext token is returned to the CLI ONCE
-- (on device-flow success or a refresh) and NEVER stored server-side; only its
-- SHA-256 hash lives here (parity with how the session cookie keeps no
-- server-side copy). Rotation-on-use inserts a new row and sets revoked_at on
-- the old one; logout (POST /auth/token/revoke) sets revoked_at. Reuse of an
-- already-rotated token is a compromise signal that revokes the whole family
-- for that user_id.
CREATE TABLE cli_refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,     -- SHA-256(refresh_token)
    expires_at TIMESTAMP NOT NULL,       -- created_at + 30d (rolling via rotation)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP NULL
);
CREATE INDEX idx_cli_refresh_tokens_user_id ON cli_refresh_tokens (user_id);
