-- Accounts for the Google-OAuth sign-in flow. Registration is implicit: the
-- first successful sign-in for an allowlisted Google account inserts the row.
--
-- google_sub is the opaque, stable Google subject identifier ("sub" claim). It
-- is the identity/lookup key the OAuth callback upserts on — it never changes
-- for an account and carries no PII, so it stays plaintext.
--
-- User-provided PII (email, display name) is ENCRYPTED AT REST: email_encrypted
-- and name_encrypted hold AES-256-GCM ciphertext (nonce-prefixed) produced by
-- internal/auth's Encryptor. There is deliberately NO plaintext email/name
-- column. email_hash is a keyed HMAC-SHA256 blind index over the normalised
-- (trim+lowercase) email — it powers the "one account per email" UNIQUE
-- constraint and equality lookups without exposing the address.
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    google_sub TEXT NOT NULL UNIQUE,
    email_encrypted BYTEA NOT NULL,
    email_hash TEXT NOT NULL UNIQUE,
    name_encrypted BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
