---
title: "Design"
weight: 1
---

# Design: Authenticated CLI login via OAuth 2.0 Device Authorization Grant

**Status:** Proposed (design only — no code in this doc)
**Author:** (design doc)
**Scope:** Add `langner login` / `logout` / `whoami` to the user CLI so a non-engineer user can authenticate the CLI to a Vercel-hosted, Google-OAuth-gated backend **without API keys**, using RFC 8628 (Device Authorization Grant). The server issues a langner-owned access JWT (24h) plus a hashed-in-DB refresh token.

---

## 1. Background — what exists today

Auth landed in three merged phases and is entirely **cookie-based** for the web app:

- **Sign-in (browser only).** `internal/auth/oauth.go` (`OAuthClient`, scopes `openid/email/profile`, Google endpoint) drives the standard authorization-code flow. `internal/server/auth_handler.go` mounts the plain-HTTP endpoints `/auth/google/login`, `/auth/google/callback`, `/auth/logout`, `/auth/me` (see `cmd/langner-server/main.go` around lines 260-264). `Login` mints a signed CSRF state (`auth.StateSigner`, `state.go`) into a temporary cookie and redirects to Google; `Callback` verifies state, exchanges the code, fetches userinfo, checks the email allowlist (`auth.IsAllowed`, `allowlist.go`), upserts the user (`UserRepository.Upsert`, keyed on the opaque `google_sub`), and sets a signed session cookie.

- **Session cookie.** `internal/auth/session.go` — `SessionSigner` produces a **stateless** HMAC-SHA256 token: `base64url(payload) + "." + base64url(hmac(payload))`, where `payload = {"uid": <int64>, "exp": <unix>}`. It carries **only** the user id and expiry — no PII. The signing key is `SESSION_SIGNING_KEY` (env-only, decoded by `auth.DecodeKey` which accepts hex/base64/raw). `sessionTTL = 30 * 24h`. Cookie name `langner_session` (`SessionCookieName`).

- **RPC gating (cookie-only today).** `internal/server/auth_middleware.go`:
  - `AuthCookieMiddleware(next, sessions)` reads the `langner_session` cookie, verifies it, and on success stores the session **and** user id in the request context (`auth.WithSession`, `auth.WithUserID`). It **never rejects** — `/auth/*` and CORS preflight pass through unauthenticated.
  - `NewAuthInterceptor()` is a Connect unary interceptor that reads `auth.SessionFromContext(ctx)`; if absent it returns `connect.CodeUnauthenticated`, otherwise it injects `auth.WithUserID(ctx, sess.UserID)` for downstream handlers.
  - Wiring: in `main.go`, the interceptor is appended to the Connect interceptor list **only when auth is enabled** (`cfg.Auth.Enabled()` ⇔ `SessionSigningKey != ""`), and `AuthCookieMiddleware` wraps the root handler. Auth requires a database (`buildAuth` fails fast if `db == nil`).

- **Users table (migration 024).** `users(id BIGSERIAL PK, google_sub TEXT UNIQUE, username TEXT UNIQUE, created_at, updated_at)`. **No email/name stored** (data minimisation); the allowlist is checked against the live Google email at callback time. `auth.UserRepository` (`user_repository.go`) has `Upsert(ctx, googleSub)` and `FindByID(ctx, id)`.

- **Per-user state (024/026/027).** `learning_logs`, `note_skip_flags`, `origin_skip_flags` carry a nullable `user_id` (migration 026); `notebooks` overlay carries ownership/visibility (027). Every RPC handler already reads `auth.UserIDFromContext` to scope reads/writes.

- **CLI today.** `cmd/langner/` is a cobra app (`main.go`) whose subcommands (`quiz`, `notebook`, `migrate`, `auth`, …) mostly load config and talk **directly to Postgres** (`openConfigAndDB`, `helpers.go`) — they do **not** call the server over RPC. The existing `langner auth` subtree (`cmd/langner/auth.go`) is **admin/tooling** (`provision`, `backfill`, `issue-test-cookie`) that operates on the DB directly; it is **not** an end-user login. A parallel effort is splitting the binary into a user `langner` and an admin `langner-admin`; **`login`/`logout`/`whoami` belong in the user `langner` binary.**

### Why the device flow (fixed decisions)

Users are not engineers, cannot SSH, and the app is Vercel-hosted. The CLI must authenticate **without API keys**. RFC 8628 Device Authorization Grant fits exactly: the CLI shows a short code + URL, the user approves in a browser using the **existing Google web sign-in**, and the CLI polls for tokens. Reusing the browser flow means no new identity surface and no secret on the user's machine beyond the issued tokens.

---

## 2. Goals / non-goals

**Goals**
- `langner login` authenticates the CLI with zero API keys, via a browser approval the user already knows (Google sign-in).
- The CLI receives a **langner-owned access JWT (24h)** + a **refresh token** (opaque; stored **hashed** server-side).
- The Connect RPC middleware **also** accepts `Authorization: Bearer <access-jwt>` **without weakening** existing cookie auth.
- Tokens are stored locally at `~/.config/langner/credentials.json`, `0600`. Silent refresh on access-token expiry; `langner logout` revokes.
- Public repo: secrets only via env; nothing personal committed.

**Non-goals**
- No change to the browser/web session cookie flow.
- No API keys, ever.
- No new identity provider — Google remains the only sign-in.
- Device-flow is for the **CLI**; the web app keeps using cookies.

---

## 3. High-level flow (RFC 8628)

```
 CLI (langner login)                Backend (/auth/device/*)             Browser + Google
 ───────────────────                ────────────────────────             ────────────────
  POST /auth/device/code  ────────► create cli_device_codes row
     (client_id)          ◄──────── { device_code, user_code,
                                       verification_uri,
                                       verification_uri_complete,
                                       expires_in, interval }
  print user_code + URL,
  open browser  ─────────────────────────────────────────────────────►  GET /auth/device
                                                                         (shows user_code form;
                                                                          if not signed in →
                                                                          existing Google login,
                                                                          then back here)
                                    POST /auth/device/approve  ◄───────  user confirms code
                                    (bind user_id → device row,
                                     set approved_at)
  poll every `interval`s:
  POST /auth/device/token ────────► if pending → 400 authorization_pending
     (device_code,                  if too fast → 400 slow_down
      grant_type=device_code)       if approved → mint access JWT +
                                       refresh token (store hash),
                                       consume device row
                          ◄──────── { access_token, token_type=Bearer,
                                       expires_in=86400, refresh_token }
  save credentials.json (0600)

 ... later, access token expired ...
  POST /auth/token/refresh ───────► verify refresh hash, rotate,
     (refresh_token)                mint new access + new refresh
                          ◄──────── { access_token, refresh_token }

 langner logout:
  POST /auth/token/revoke ────────► mark refresh row revoked_at
```

Every subsequent RPC from the CLI carries `Authorization: Bearer <access-jwt>`.

---

## 4. New HTTP endpoints

Mounted **only when auth is enabled**, exactly like the existing `/auth/*` block in `main.go`, and served **outside** the Connect interceptor (so an unauthenticated CLI can reach them). They live next to the current handlers, e.g. a new `internal/server/device_auth_handler.go` (`DeviceAuthHandler`) wired in `buildAuth`. All bodies are JSON (the CLI is not a browser form), except the human-facing approval page.

Base URL note: the CLI targets the deployed API origin (e.g. `https://api.langner.app`). Because Vercel hosts the frontend, the **backend** origin is a separate configured host; the CLI stores/uses that as its server base URL (see §7).

### 4.1 `POST /auth/device/code` — device authorization request (RFC 8628 §3.1/§3.2)

Request (JSON):
```json
{ "client_id": "langner-cli" }
```
(`scope` is omitted — langner has a single scope.)

Response `200`:
```json
{
  "device_code": "<43+ char opaque, base64url>",
  "user_code": "WDJB-MJHT",
  "verification_uri": "https://api.langner.app/auth/device",
  "verification_uri_complete": "https://api.langner.app/auth/device?user_code=WDJB-MJHT",
  "expires_in": 900,
  "interval": 5
}
```
Server creates a `cli_device_codes` row (`user_id` NULL, `approved_at` NULL). `expires_in = 900` (15 min). `interval = 5` s.

### 4.2 `GET /auth/device` + `POST /auth/device/approve` — browser approval (reuses Google sign-in)

- `GET /auth/device?user_code=…` renders a minimal HTML page: a field pre-filled with `user_code` and an **Approve** button. If the request has **no valid `langner_session` cookie**, the page links to `/auth/google/login?next=/auth/device?user_code=…` so the user signs in with the **existing** Google flow and returns here. (Reuse `AuthCookieMiddleware`, which already populates the session context for `/auth/*`.)
- `POST /auth/device/approve` (form or JSON `{ "user_code": "WDJB-MJHT" }`) requires a valid session cookie (`auth.SessionFromContext`). It looks up the device row by `user_code`, checks it is unexpired and unapproved, sets `user_id = <session user id>` and `approved_at = now()`. Shows a "You can return to your terminal" confirmation. A deny action sets a terminal `access_denied` state (see below).

Security: `user_code` lookup is rate-limited and constant-time-compared; an expired/consumed code returns a generic error page (no enumeration signal).

### 4.3 `POST /auth/device/token` — token poll (RFC 8628 §3.4/§3.5)

Request:
```json
{ "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
  "device_code": "<opaque>",
  "client_id": "langner-cli" }
```

Responses:
- **Pending** — `400` `{ "error": "authorization_pending" }` (row exists, `approved_at` NULL).
- **Polling too fast** — `400` `{ "error": "slow_down" }` when the caller polls faster than `interval` (server tracks `last_polled_at`; per RFC the CLI must then add 5s to its interval).
- **Expired** — `400` `{ "error": "expired_token" }` when `now() > expires_at`.
- **Denied** — `400` `{ "error": "access_denied" }` when the user rejected the request.
- **Unknown/invalid device_code** — `400` `{ "error": "expired_token" }` (do not distinguish unknown from expired — avoids enumeration).
- **Success** — `200`:
```json
{
  "access_token": "<langner access JWT>",
  "token_type": "Bearer",
  "expires_in": 86400,
  "refresh_token": "<opaque 32-byte base64url>"
}
```
On success the device row is **consumed** (deleted or marked terminal so the same `device_code` cannot mint a second token), a `cli_refresh_tokens` row is inserted with the **hash** of the refresh token, and the access JWT is signed (§6).

### 4.4 `POST /auth/token/refresh` — refresh (rotation-on-use)

Request: `{ "grant_type": "refresh_token", "refresh_token": "<opaque>" }`
Server: SHA-256 the presented token, look up a **non-revoked, unexpired** `cli_refresh_tokens` row by `token_hash`. On match: **rotate** — mark the old row `revoked_at = now()`, insert a new refresh row (new hash), mint a fresh access JWT. Response identical to the success shape in §4.3.
Failure (no match / revoked / expired): `400` `{ "error": "invalid_grant" }`.
**Reuse detection:** presenting an already-revoked (rotated-away) refresh token is treated as compromise — revoke the entire token family for that `user_id` and return `invalid_grant` (forces a fresh `langner login`).

### 4.5 `POST /auth/token/revoke` — revoke (logout)

Request: `{ "refresh_token": "<opaque>" }` (RFC 7009 style). Server hashes it, sets `revoked_at = now()` on the matching row. Always returns `200` (idempotent; no signal whether the token existed). This is what `langner logout` calls; the access JWT is short-lived and simply expires.

---

## 5. Database changes

Next available migration number: existing tree goes up to **027**; **028 is claimed** by an in-flight `NOT NULL` change, so this feature should take **029** (device codes) and **030** (refresh tokens) — split into two files so they can land/rollback independently. Confirm the next free numbers at implementation time (golang-migrate orders by version; never insert a lower number after a higher one has run — see the 025-gap note in 027's header).

Both tables mirror the existing style: `BIGSERIAL` PK, `TIMESTAMP` columns, `set_updated_at` trigger where an `updated_at` exists, `ON DELETE CASCADE` to `users(id)`.

### 5.1 `029_cli_device_codes`

```sql
-- Short-lived OAuth 2.0 Device Authorization Grant (RFC 8628) records for the
-- CLI login. A row is created by POST /auth/device/code (user_id NULL until the
-- browser approves), bound to a user by POST /auth/device/approve, and consumed
-- by POST /auth/device/token. Rows are ephemeral: a sweeper deletes expired ones.
CREATE TABLE cli_device_codes (
    id             BIGSERIAL PRIMARY KEY,
    device_code_hash TEXT NOT NULL UNIQUE, -- SHA-256 of the opaque device_code (never store plaintext)
    user_code      TEXT NOT NULL UNIQUE,   -- human code shown in the CLI, e.g. WDJB-MJHT
    user_id        BIGINT NULL REFERENCES users(id) ON DELETE CASCADE, -- set on approval
    status         VARCHAR(16) NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','approved','denied','consumed')),
    last_polled_at TIMESTAMP NULL,         -- for slow_down enforcement
    approved_at    TIMESTAMP NULL,
    expires_at     TIMESTAMP NOT NULL,     -- created_at + 15m
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cli_device_codes_expires_at ON cli_device_codes (expires_at);
```
Note: store the **hash** of `device_code` (it is a bearer secret during polling), and look up by `device_code_hash`. `user_code` is stored plaintext because it is deliberately shown to the user and typed into the browser.

### 5.2 `030_cli_refresh_tokens`

```sql
-- Hashed CLI refresh tokens. The plaintext token is returned to the CLI ONCE and
-- NEVER stored server-side; only its SHA-256 hash lives here (parity with how the
-- session cookie stores no server-side copy). Rotation-on-use inserts a new row
-- and sets revoked_at on the old one; logout sets revoked_at.
CREATE TABLE cli_refresh_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,     -- SHA-256(refresh_token)
    expires_at   TIMESTAMP NOT NULL,       -- created_at + 30d (rolling via rotation)
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at   TIMESTAMP NULL
);
CREATE INDEX idx_cli_refresh_tokens_user_id ON cli_refresh_tokens (user_id);
```

A new repository `auth.CLIDeviceCodeRepository` and `auth.CLIRefreshTokenRepository` (in `internal/auth/`, `sqlx`-based, mirroring `UserRepository`) encapsulate create/lookup/consume/rotate/revoke. Down-migrations `DROP TABLE IF EXISTS …`.

A periodic sweep (`DELETE FROM cli_device_codes WHERE expires_at < now()` and optional purge of long-expired revoked refresh rows) runs either as a lightweight goroutine ticker in the server or a `langner-admin` maintenance command — cheap, and the `expires_at` index supports it.

---

## 6. Access JWT

### 6.1 Shape

A compact JWT (HS256), claims:
```json
{
  "iss": "langner",
  "sub": "<user id as string>",
  "jti": "<random 16 bytes, base64url>",
  "iat": <unix>,
  "exp": <iat + 86400>          // 24h
}
```
`sub` is the numeric `users.id` (same identity the cookie's `uid` carries). `jti` gives per-token identity for future denylisting/audit. No email/name (data minimisation, matching the cookie's no-PII rule). Aud/scope omitted (single audience, single scope).

### 6.2 Signing key — recommendation

**Use a dedicated `CLI_TOKEN_SIGNING_KEY`, not `SESSION_SIGNING_KEY`.** Justification:
- **Blast-radius isolation.** The two tokens have different lifetimes (cookie 30d vs access JWT 24h), different transports (browser cookie vs CLI bearer), and different revocation stories. A dedicated key lets us rotate one without invalidating the other.
- **Format divergence.** The cookie today is a **custom HMAC envelope** (`base64url(json).base64url(hmac)`), not a JWT. Introducing a JWT under the same key mixes two token formats on one secret — easy to confuse in verification code. Separate keys keep `SessionSigner.Verify` and the new JWT verifier from ever being pointed at each other's tokens.
- **Least surprise / defense in depth.** If the CLI token verifier had a bug, we would not want it reachable with the cookie key.

Wire it exactly like `SESSION_SIGNING_KEY`: add `auth.session_cli_token_signing_key` (or `auth.cli_token_signing_key`) to `AuthConfig`, `v.BindEnv("auth.cli_token_signing_key", "CLI_TOKEN_SIGNING_KEY")` in `config.go`, decode with `auth.DecodeKey`. **Fallback for single-key deployments:** if `CLI_TOKEN_SIGNING_KEY` is unset, fall back to `SESSION_SIGNING_KEY` so a minimal deployment still works — but document the dedicated key as recommended. Never commit either (public repo).

A small `auth.CLITokenSigner` (new file `internal/auth/cli_token.go`) wraps sign/verify. Use a vetted JWT library or a tiny hand-rolled HS256 (the codebase already hand-rolls HMAC in `session.go`); given HS256's simplicity and the existing pattern, a hand-rolled signer keeps dependencies minimal and is acceptable — but a maintained library (e.g. `golang-jwt/jwt`) is preferable for standards conformance (base64url header, `alg` pinning to reject `none`). Decide in review (§9).

### 6.3 Verification path (bearer alongside cookie — the key requirement)

Today `AuthCookieMiddleware` is the only thing that populates the session/user-id context, and it is cookie-only. To accept a bearer **without weakening cookie auth**, extend the *middleware*, not the interceptor:

- In `AuthCookieMiddleware` (rename conceptually to "auth context middleware"; keep the function or add a sibling `AuthBearerMiddleware` chained after it), after the existing cookie check, **if no session was established from the cookie**, check for an `Authorization: Bearer <token>` header. If present and `CLITokenSigner.Verify` succeeds (valid signature, `exp` not passed, `iss == "langner"`), populate the context with `auth.WithSession(ctx, Session{UserID: sub, ExpiresAt: exp})` and `auth.WithUserID`.
- The `NewAuthInterceptor()` is **unchanged** — it still only reads `auth.SessionFromContext`. Because either the cookie path or the bearer path fills that context, both authenticate uniformly and every downstream handler's `auth.UserIDFromContext` keeps working with no per-handler change.
- **Cookie precedence / no weakening:** the cookie branch runs first and is untouched; the bearer branch only *adds* a second way to populate the same context and only runs when the cookie is absent/invalid. A malformed/expired bearer simply leaves the context empty → the interceptor rejects with `CodeUnauthenticated`, exactly as an anonymous request does today. No existing cookie behavior changes.
- CORS: bearer requests come from the CLI (no browser Origin), so the credentialed-CORS logic is irrelevant to them; leave it as-is. The `Authorization` header must be added to `Access-Control-Allow-Headers` only if a browser ever needs to send a bearer (not required for the CLI).

This is the entire "server also accepts Bearer" change: **one middleware, no interceptor or handler changes.**

---

## 7. CLI side (`langner` user binary)

New cobra commands under `cmd/langner/` (sibling files, e.g. `login.go`), registered in `main.go`'s `AddCommand(...)` list. These talk to the **server over HTTP**, unlike the DB-direct admin commands.

### 7.1 Commands

- **`langner login [--server <url>]`**
  1. `POST {server}/auth/device/code` → get `user_code`, `verification_uri(_complete)`, `interval`, `expires_in`.
  2. Print, e.g.:
     ```
     To sign in, open:  https://api.langner.app/auth/device
     and enter code:    WDJB-MJHT
     (waiting for approval…)
     ```
     Attempt to open the browser automatically (best-effort `xdg-open`/`open`/`start`); always print the URL+code as fallback (headless-safe).
  3. Poll `POST {server}/auth/device/token` every `interval`s; honor `authorization_pending` (keep waiting), `slow_down` (add 5s), `expired_token`/`access_denied` (abort with a clear message), success (save tokens).
  4. Write `credentials.json` (§7.2), print `Logged in as <username>` (fetch via a `whoami`/`/auth/me`-style call using the new bearer).

- **`langner logout`** — read the stored refresh token, `POST {server}/auth/token/revoke`, then delete `credentials.json` (best-effort even if the network call fails). Print `Logged out.`

- **`langner whoami`** — ensure a valid access token (refresh if needed, §7.3), call an identity endpoint (reuse `/auth/me`, which returns `{authenticated, username}` and already reads the session context — now also satisfiable by the bearer), print the username. If not logged in, print guidance to run `langner login`.

### 7.2 Token storage

Path: **`~/.config/langner/credentials.json`** (respect `$XDG_CONFIG_HOME` if set → `$XDG_CONFIG_HOME/langner/credentials.json`). Create the dir `0700`, the file `0600`. Shape:
```json
{
  "server_url": "https://api.langner.app",
  "access_token": "<jwt>",
  "access_token_expires_at": "2026-09-21T12:00:00Z",
  "refresh_token": "<opaque>"
}
```
A small `internal/cli/credentials.go` (or `internal/clitoken/`) owns load/save/delete with the perm bits enforced on write (and a warning if it finds looser perms). Never log token values.

### 7.3 Auto-refresh + attaching the bearer

- A shared HTTP round-tripper / Connect interceptor for the CLI attaches `Authorization: Bearer <access_token>` to every server call.
- **Proactive refresh:** if `access_token_expires_at` is within a small skew (e.g. < 60s) of now, refresh before the call.
- **Reactive refresh on 401:** if a call returns `401`/`CodeUnauthenticated`, call `/auth/token/refresh` once, persist the rotated pair, and retry the original request. If refresh fails (`invalid_grant`), delete credentials and tell the user to run `langner login`.
- Existing/future CLI RPC commands opt in simply by constructing their Connect client with this interceptor; DB-direct admin commands (which don't hit the server) are unaffected.

Because the CLI commands today are DB-direct, the bearer plumbing is only needed for commands that call the **server**. This design adds the plumbing so future user-facing RPC commands (e.g. a CLI quiz) authenticate transparently.

---

## 8. Security review

- **Device code entropy.** `device_code` = 32 random bytes (`crypto/rand`) base64url (~43 chars), stored **hashed** (SHA-256). It is a bearer secret during the polling window; hashing at rest means a DB read cannot mint tokens.
- **`user_code` format.** 8 chars from an unambiguous alphabet (RFC 8628 recommends excluding easily-confused chars; use Crockford-style `BCDFGHJKLMNPQRSTVWXZ` + digits minus `0/1`), rendered `XXXX-XXXX`. ~20^8 ≈ 2.5e10 space; combined with a 15-min expiry, single-active-code-per-poll, and rate limiting, brute force on the approval page is impractical. Look it up case-insensitively but compare in constant time.
- **Expiry / rotation.** Device codes expire in **15 min**. Access JWT **24h** (fixed). Refresh tokens **30 days**, **rotated on every use**; the old token is revoked immediately, and **reuse of a rotated token triggers family-wide revocation** (compromise signal). Logout revokes.
- **No plaintext secrets at rest.** Neither the refresh token nor the device code is stored in plaintext server-side (only SHA-256 hashes) — the same posture as the stateless cookie storing no server copy. The plaintext refresh token exists only in the CLI's `0600` file.
- **Revocation.** `langner logout` → `/auth/token/revoke`. Access JWTs are stateless and **not** individually revocable before `exp`; the 24h lifetime bounds exposure. If instant kill-switch is needed later, add a `jti`/user denylist checked in the bearer middleware (the `jti` claim is already provisioned).
- **Clock skew.** Verify `exp` with a small leeway (± a few seconds). CLI refreshes proactively at <60s remaining so a slightly-fast client clock never sends a just-expired token.
- **Binding approval to the right user.** Approval requires a valid `langner_session` cookie and only sets `user_id` from that server-verified session — the browser cannot approve a device for someone else.
- **Enumeration resistance.** `/auth/device/token` returns `expired_token` for both unknown and expired `device_code`; the approval page shows a generic error for bad/consumed `user_code`. Rate-limit `/auth/device/*`.
- **Transport.** All endpoints must be HTTPS in production (Vercel/host-terminated TLS); the CLI should refuse a non-HTTPS `server_url` unless `localhost` (dev).
- **Allowlist still applies.** Approval goes through the existing Google sign-in, so the email allowlist (`auth.IsAllowed`) gates who can ever get a session to approve with — CLI access inherits it for free.
- **Public-repo secret rules.** `CLI_TOKEN_SIGNING_KEY` (and `SESSION_SIGNING_KEY`, `GOOGLE_CLIENT_SECRET`) are **env-only**, bound in `config.go`, never written to YAML or committed. `config.example.yml` documents the env var names only (as it already does for the session key). No personal data in examples/tests (use neutral placeholders).

---

## 9. Open questions / decisions still needed

1. **JWT library vs hand-rolled HS256.** Recommend a maintained library (`golang-jwt/jwt/v5`) for `alg` pinning and standards conformance; the repo's minimalist style could justify hand-rolling. **Decide before implementation.**
2. **Exact migration numbers.** This doc assumes 028 is taken by the in-flight `NOT NULL` change → use **029/030**. Re-confirm the next free versions at implementation time.
3. **Signing-key fallback policy.** Confirm whether `CLI_TOKEN_SIGNING_KEY` should fall back to `SESSION_SIGNING_KEY` when unset (proposed: yes, for single-key deployments) or be **required** when CLI login is enabled (stricter).
4. **Device-code cleanup mechanism.** Server goroutine ticker vs `langner-admin` maintenance command vs a DB TTL job — pick one.
5. **`client_id` semantics.** Single fixed `"langner-cli"` public client (no secret, per RFC 8628 for native apps) — confirm we don't want per-build client ids.
6. **Server base URL discovery for the CLI.** How does a non-engineer learn the `--server` value? Options: bake a production default into the binary, ship it in a public `config.example.yml`, or prompt on first `login`. Recommend a compiled-in default (`https://api.langner.app`) overridable by `--server`/env.
7. **`/auth/me` reuse for `whoami`.** Confirm we extend `/auth/me` (already returns username) rather than adding a new endpoint — it already reads the session context that the bearer middleware now fills.
8. **Rate-limiting layer.** Where do `/auth/device/*` rate limits live — app middleware, or rely on the hosting platform (Vercel/edge)?
9. **Two-binary split ordering.** `login`/`logout`/`whoami` land in the user `langner` binary; sequence this against the in-flight split so the commands don't get placed in `langner-admin` by mistake.

---

## 10. Migration / rollout plan

1. **Migrations 029/030** (`cli_device_codes`, `cli_refresh_tokens`) + repositories. Additive, no change to existing tables. Down-migrations drop the new tables only.
2. **Config + key.** Add `CLI_TOKEN_SIGNING_KEY` binding (env-only); document in `config.example.yml` (name only). CLI login is enabled whenever auth is enabled (same `cfg.Auth.Enabled()` gate).
3. **Server endpoints** (`DeviceAuthHandler`) mounted in the existing `if authSetup != nil { … }` block; **bearer verification added to the auth-context middleware** (§6.3) — the interceptor and handlers are untouched.
4. **CLI commands** `login`/`logout`/`whoami` + credentials store + auto-refresh interceptor, in the user `langner` binary.
5. **Example notebooks + real-config integration test** (per repo rule `verify-data-features-with-example-notebooks.md`): construct the server the way `main.go` does from `config.example.yml` (auth enabled, DB), drive the full device flow end-to-end against a real Postgres — request a code, approve it via a synthesized session (reuse `issue-test-cookie`/`SessionSigner` to mint the approving session), poll to success, call a gated RPC with the returned bearer and observe it authenticates, refresh (assert rotation + old-token reuse revokes the family), and revoke (assert subsequent refresh fails). Also seed/verify the bearer path does **not** weaken cookie auth (a cookie'd request still works; an anonymous one still 401s).
6. **Rollback:** feature is additive and gated; disabling is dropping the new endpoints/commands and rolling back 030→029. Existing cookie auth is entirely independent.
7. **Verification gate (repo rule `no-unverified-main-merges`):** do not propose merge until CI is green on the exact head SHA **and** the device flow has been exercised end-to-end against a real Postgres (browser-approval simulated via the session signer) with the bearer observed authenticating a real RPC. Anything that can only run against the live Vercel/Postgres deployment is called out as unverified until exercised there.
```
