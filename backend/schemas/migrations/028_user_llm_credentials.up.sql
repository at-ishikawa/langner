-- Per-user LLM provider credentials for quiz grading. There is deliberately NO
-- system-wide server API key: each user registers their OWN provider + API key
-- (+ optional model), and grading resolves the LLM client per request from the
-- signed-in user's credential.
--
-- One active credential per user (user_id PRIMARY KEY): switching provider or
-- rotating the key overwrites the row. The API key is ENCRYPTED AT REST —
-- api_key_encrypted holds AES-256-GCM ciphertext (nonce-prefixed) produced by
-- internal/auth's Encryptor (the same CREDENTIAL_ENCRYPTION_KEY that encrypts
-- user PII). There is NO plaintext key column, and the key is never returned to
-- the client; it is decrypted in-process only for the outbound LLM call.
CREATE TABLE user_llm_credentials (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(16) NOT NULL,
    api_key_encrypted BYTEA NOT NULL,
    model VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER user_llm_credentials_set_updated_at BEFORE UPDATE ON user_llm_credentials
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
