-- Short-lived OAuth 2.0 Device Authorization Grant (RFC 8628) records for the
-- CLI login (`langner login`). A row is created by POST /auth/device/code
-- (user_id NULL until the browser approves), bound to a user when the approving
-- browser completes the Google sign-in, and consumed by POST /auth/device/token.
-- Rows are ephemeral: a sweeper deletes expired ones.
--
-- The opaque device_code is a bearer secret during the polling window, so only
-- its SHA-256 hash is stored (device_code_hash) and looked up — a DB read can
-- never mint tokens. user_code is stored plaintext because it is deliberately
-- shown to the user and typed into the browser approval page.
--
-- Migration numbering: 028 is the in-flight user_id NOT NULL change; this
-- feature takes 029 (device codes) and 030 (refresh tokens), split so they can
-- land/rollback independently. golang-migrate orders by version — never insert
-- a lower number after a higher one has run.
CREATE TABLE cli_device_codes (
    id               BIGSERIAL PRIMARY KEY,
    device_code_hash TEXT NOT NULL UNIQUE,   -- SHA-256 of the opaque device_code (never store plaintext)
    user_code        TEXT NOT NULL UNIQUE,   -- human code shown in the CLI, e.g. WDJB-MJHT
    user_id          BIGINT NULL REFERENCES users(id) ON DELETE CASCADE, -- set on approval
    status           VARCHAR(16) NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'approved', 'denied', 'consumed')),
    last_polled_at   TIMESTAMP NULL,         -- for slow_down enforcement
    approved_at      TIMESTAMP NULL,
    expires_at       TIMESTAMP NOT NULL,     -- created_at + 15m
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cli_device_codes_expires_at ON cli_device_codes (expires_at);
