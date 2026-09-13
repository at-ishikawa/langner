-- Accounts for the Google-OAuth sign-in flow. Registration is implicit: the
-- first successful sign-in for an allowlisted Google account inserts the row.
--
-- google_sub is the opaque, stable Google subject identifier ("sub" claim) —
-- the identity/lookup key the OAuth callback upserts on. It never changes for
-- an account and carries no PII.
--
-- No email or display name is stored (data minimisation): the allowlist is
-- checked against the LIVE email Google returns at callback time, never a
-- stored copy, so nothing here is PII. username is an auto-generated, unique,
-- user-facing handle (editable via a later change) — the only human-readable
-- identifier kept.
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    google_sub TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
