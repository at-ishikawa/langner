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

> **Architecture update (supersedes the original cookie-based framing below).**
> The frontend (Next.js) and the RPC API (Go) will run on **separate origins** once
> hosted on Vercel (static/SSR frontend on Vercel, the Go connect-RPC server on its
> own host). A session **cookie is cross-site** in that topology — it needs
> `SameSite=None; Secure` and is subject to third-party-cookie blocking, so it is
> the wrong primitive. The **web app therefore uses the SAME bearer access-token
> model as the CLI**: one unified langner-token auth for both clients. The cookie
> flow is **removed**, not kept in parallel.

**Goals**
- **One auth model for both web and CLI: a langner-owned bearer access token.** `langner login` (CLI, device flow) and the web sign-in both end with a langner **access JWT (24h)** + a **refresh token** (opaque; stored **hashed** server-side); every RPC — from the browser or the CLI — carries `Authorization: Bearer <access-jwt>`.
- **Auth is ALWAYS mounted and ALWAYS required.** There is no opt-in/opt-out: no `Auth.Enabled()` gate, no "run ungated" mode, no `AUTH=0`. The server fails fast at startup if its token-signing key is absent. Cross-origin requests authenticate by bearer, never by cookie.
  - **Consequence (breaking): every server run now needs Postgres + Google creds + `TOKEN_SIGNING_KEY`.** `buildAuth` already fails fast when `db == nil` (`main.go:316`), and auth being mandatory removes the YAML-only / no-DB dev mode (`main.go:109-137`) and `AUTH=0` (`Makefile:44,54`). This is an accepted consequence of the multi-user + Vercel direction (per-user state lives in the DB anyway). `make dev` must always generate `TOKEN_SIGNING_KEY` (it already generates a signing key via `.dev-auth.env`) and start Postgres; the YAML-only content-authoring loop is replaced by the CLI push path (see the per-user-notebooks proposal). This is item M1 in §11.
- Zero API keys — a browser approval the user already knows (Google sign-in) is the only credential ceremony.
- Tokens are stored client-side (CLI: a JSON file, `0600`; web: see §8). Silent refresh on access-token expiry; `logout` revokes server-side.
- **No fallbacks** — not for signing keys (the token key is its own required secret) and not across servers (the CLI store is keyed per server; a missing entry is an error, never a silent default).
- Public repo: secrets only via env; nothing personal committed.

**Non-goals**
- No API keys, ever.
- No new identity provider — Google remains the only upstream sign-in.
- No session cookie — it is removed, not merely deprecated. (The only remaining browser cookie use is the short-lived OAuth CSRF `state` during the Google redirect, which is same-site to the API host and not an app session.)

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

Because there is **no app session cookie** (§8), the verified Google identity exists **only within the callback request** — there is nothing to carry it to a later independent "approve" POST. So **approval is bound inside the OAuth callback**, not as a separate stateless request:

- `GET /auth/device?user_code=…` renders a minimal HTML confirmation page that shows *exactly* what is being authorized ("Approve CLI access for code `WDJB-MJHT`?") and, on Approve, redirects to `GET /auth/google/login` with the **`user_code` bound into the signed `state`** (see M5/§6.2 — `statePayload` gains `next` and optional `user_code`, so the code survives the Google round-trip tamper-proof; a raw query param would be an open-redirect / injection vector).
- `GET /auth/google/callback` — after it verifies `state`, exchanges the code, checks the allowlist, and upserts the user (`auth_handler.go:87-150`), **if `state.user_code` is present** it looks up the device row, checks it is unexpired and unapproved, and sets `user_id = <this just-verified identity>` + `approved_at = now()` **in the same request** — this IS the approval. It then renders "You can return to your terminal." (No `/auth/device/approve` endpoint; there is no principal outside the callback to power one.) A deny link hits a small endpoint that sets the terminal `access_denied` state for the `user_code` (no identity needed to deny).

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
`sub` is the numeric `users.id`. `jti` gives per-token identity for future denylisting/audit. No email/name (data minimisation, keeping the no-PII rule migration 024 established). Aud/scope omitted (single audience, single scope).

### 6.2 Signing key — one required key

There is exactly **one** auth signing key now: `TOKEN_SIGNING_KEY`, which signs the access JWT for **both** the web SPA and the CLI (they share the token model, §2/§8). The old `SESSION_SIGNING_KEY` is **removed** with the cookie — there is no second token format and no separate cookie key to isolate from. (Refresh tokens are opaque random strings stored hashed, not signed, so they need no key.)

Wire it: add `auth.token_signing_key` to `AuthConfig`, `v.BindEnv("auth.token_signing_key", "TOKEN_SIGNING_KEY")` in `config.go`, decode with `auth.DecodeKey`. **No fallback.** The token signing key is the single required auth secret; the server **fails fast at startup if it is unset** (auth is always mounted, §2). Because the session cookie is removed, `SESSION_SIGNING_KEY` goes away entirely — there is only one signing key now, and it signs the access JWT. (If a deployment set both today, the migration is: drop `SESSION_SIGNING_KEY`, set `TOKEN_SIGNING_KEY`.) Never commit it (public repo).

**The OAuth `state` signer reuses `TOKEN_SIGNING_KEY`.** Today `buildAuth` builds `StateSigner` from `cfg.SessionSigningKey` (`main.go:323`); with that key gone, `StateSigner` keys off `TOKEN_SIGNING_KEY`. And **`statePayload` (`state.go:22`) gains two fields — `next` (the post-callback redirect target) and an optional `user_code`** — so the device `user_code` and the SPA return URL survive the Google round-trip **inside the signed, tamper-proof state** (M3/M5). The callback MUST allowlist `next` against the configured frontend origin(s) before redirecting (open-redirect guard); `Login` (`auth_handler.go:64`), which today ignores query params, reads `next`/`user_code` and folds them into the signed state.

A small `auth.CLITokenSigner` (new file `internal/auth/cli_token.go`) wraps sign/verify. Use a vetted JWT library or a tiny hand-rolled HS256 (the codebase already hand-rolls HMAC in `session.go`); given HS256's simplicity and the existing pattern, a hand-rolled signer keeps dependencies minimal and is acceptable — but a maintained library (e.g. `golang-jwt/jwt`) is preferable for standards conformance (base64url header, `alg` pinning to reject `none`). Decide in review (§9).

### 6.3 Verification path (bearer is the ONLY path)

The `langner_session` cookie and `AuthCookieMiddleware`'s cookie branch are **removed**. A single **`AuthBearerMiddleware`** populates the session/user-id context from `Authorization: Bearer <token>`:

- `AuthBearerMiddleware(next)` reads the `Authorization: Bearer <token>` header. If present and `TokenSigner.Verify` succeeds (valid signature, `exp` not passed, `iss == "langner"`), it populates the context with `auth.WithSession(ctx, Session{UserID: sub, ExpiresAt: exp})` and `auth.WithUserID`. Absent/invalid → context stays empty. It **never rejects** (so `/auth/*` still passes through); the interceptor does the rejecting.
- `NewAuthInterceptor()` is **unchanged** — it still only reads `auth.SessionFromContext` and rejects with `CodeUnauthenticated` when empty. This is the always-on gate; there is no `Auth.Enabled()` guard around appending it (auth is always mounted, §2). Every handler's `auth.UserIDFromContext` keeps working untouched.
- **CORS matters now** (unlike the CLI-only original): the browser sends the bearer from a *different origin*, so `Authorization` MUST be in `Access-Control-Allow-Headers`, the API's allowed-origins list must include the Vercel frontend origin(s), and — because auth is a header, not a cookie — the response no longer needs `Access-Control-Allow-Credentials` for auth. See §8.

This removes a whole class of cross-site-cookie problems: there is no cookie to be `SameSite`-blocked, and no CSRF surface on the RPC (a bearer is not sent automatically by the browser the way a cookie is).

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

### 7.2 Token storage — per-server, no fallback

The CLI must hold credentials for **multiple servers** — a **dev** server (e.g. `http://localhost:8080`) and a future **production** server (which does not exist yet) — at the same time, without one leaking into the other. The store is therefore a map **keyed by server URL**, and the active server is selected explicitly per invocation (a `--server` flag and/or a `LANGNER_SERVER` env var, with a `login`-set default). There is **no fallback**: if there is no entry for the selected server, commands that need auth fail with "not logged in to `<server>` — run `langner login --server <server>`", never silently reuse another server's token.

Path: **`~/.config/langner/credentials.json`** (respect `$XDG_CONFIG_HOME` → `$XDG_CONFIG_HOME/langner/credentials.json`). Dir `0700`, file `0600`. Shape:
```json
{
  "default_server": "http://localhost:8080",
  "servers": {
    "http://localhost:8080": {
      "access_token": "<jwt>",
      "access_token_expires_at": "2026-09-21T12:00:00Z",
      "refresh_token": "<opaque>"
    },
    "https://api.langner.app": {
      "access_token": "<jwt>",
      "access_token_expires_at": "2026-09-21T12:00:00Z",
      "refresh_token": "<opaque>"
    }
  }
}
```
Server-URL resolution order (first match wins, **no fallback beyond it**): explicit `--server` flag → `LANGNER_SERVER` env → `default_server` in the store. `langner login --server <url>` adds/replaces that server's entry and (if it's the first, or `--set-default`) sets `default_server`; `langner logout --server <url>` revokes and removes just that entry. A small `internal/cli/credentials.go` (or `internal/clitoken/`) owns load/save/delete keyed by server, enforces the perm bits on write (warns on looser perms), and never logs token values. Tokens from different servers are signed by different keys, so a token is only ever presented to the server it was issued by.

### 7.3 Auto-refresh + attaching the bearer

- A shared HTTP round-tripper / Connect interceptor for the CLI attaches `Authorization: Bearer <access_token>` to every server call.
- **Proactive refresh:** if `access_token_expires_at` is within a small skew (e.g. < 60s) of now, refresh before the call.
- **Reactive refresh on 401:** if a call returns `401`/`CodeUnauthenticated`, call `/auth/token/refresh` once, persist the rotated pair, and retry the original request. If refresh fails (`invalid_grant`), delete credentials and tell the user to run `langner login`.
- Existing/future CLI RPC commands opt in simply by constructing their Connect client with this interceptor; DB-direct admin commands (which don't hit the server) are unaffected.

Because the CLI commands today are DB-direct, the bearer plumbing is only needed for commands that call the **server**. This design adds the plumbing so future user-facing RPC commands (e.g. a CLI quiz) authenticate transparently.

---

## 8. Web app auth (token-based, cross-origin)

The browser SPA authenticates with the **same langner access/refresh tokens** as the CLI — no session cookie. This is what makes the Vercel split-origin topology work: the frontend on Vercel talks to the API on another host purely via `Authorization: Bearer`.

**Obtaining a token (browser).** The web sign-in is NOT the device flow (that is for the headless CLI). It is the ordinary Google authorization-code flow, ending in a token mint:

1. Frontend sends the user to `GET {api}/auth/google/login?next=<frontend-url>`.
2. `/auth/google/callback` verifies the OAuth `state` (short-lived, `SameSite=Lax` cookie **same-site to the API host** — this is the only browser cookie left, and it is not an app session), exchanges the code, checks the email allowlist, upserts the user — then **mints a langner access JWT + refresh token** (same issuer/keys as the CLI path, §6) and hands them to the SPA.
3. **Token delivery to the SPA.** The **access token** (short, 24h) is delivered to the SPA to hold in memory — via a fragment redirect `{frontend}/auth/callback#access_token=…` (fragment is never sent to a server) or a `postMessage` from a same-site-to-API poster page. The **refresh token is NEVER delivered to JS**: the callback sets it as a `Secure; HttpOnly; SameSite=None` cookie **scoped to the API origin and path `/auth/token`** (so it is sent only to the refresh/revoke endpoints, never to RPCs). The refresh token therefore does **not** appear in the URL/history (fixing the fragment-leak) and is unreadable by page script.

**Storing a token (browser) — refresh token in an API-scoped HttpOnly cookie (proposed default).** XSS is the threat model once auth is a bearer. Keeping the refresh token in `localStorage`/JS would mean an XSS = **persistent** account takeover (the attacker just keeps rotating) — strictly worse than the removed HttpOnly session cookie. So the refresh token lives in the **HttpOnly API-scoped cookie** above (script-unreadable; XSS blast radius bounded to the 24h in-memory access token), plus rotation-on-use (§9) and a strict CSP.
  - **Tension to resolve (M4):** that refresh cookie is itself `SameSite=None` and cross-site to the frontend — the very property §2 cited to abandon the *session* cookie. The difference we are betting on: it is **refresh-only** (one endpoint, not every request), so a third-party-cookie-blocked browser degrades to "re-run Google sign-in when the access token expires" (a 24h-cadence annoyance) rather than "app broken" — whereas a blocked *session* cookie breaks *every* RPC. If even that is unacceptable, the fallback is refresh-token-in-memory (lost on reload → sign in again each visit). **This is the one security decision to confirm before implementing** (§10 Q3).

**Using it.** The SPA's Connect client attaches `Authorization: Bearer <access_token>` to every RPC and refreshes via `/auth/token/refresh` exactly like the CLI (§7.3). CORS on the API must allow the Vercel origin(s) and the `Authorization` header (§6.3).

**Consequence for existing code.**
- **Backend:** `internal/auth/session.go` (`SessionSigner`, `langner_session`) and `AuthCookieMiddleware`'s cookie branch are **deleted**; `/auth/google/callback` (`auth_handler.go:87`) stops `Set-Cookie`-ing a session and instead mints the access token (fragment/postMessage) + sets the API-scoped HttpOnly refresh cookie; `/auth/logout` (`auth_handler.go:153`) becomes `/auth/token/revoke` semantics (revoke the presented refresh token, clear the refresh cookie); `/auth/me` reads the bearer-filled context (§6.3).
- **Frontend (this IS a change, not "unchanged"):** the shared client and `src/lib/auth.ts` (`getMe`/`logout`) currently use `credentials:"include"` (cookie) — they must instead attach `Authorization: Bearer <access_token>` from the in-memory token and do reactive refresh-on-401 (calling `/auth/token/refresh`, which succeeds via the HttpOnly refresh cookie). Web **logout** calls `/auth/token/revoke`. `AppHeader`'s `/auth/me` username fetch works once it sends the bearer. A first-load with no in-memory access token triggers a silent `/auth/token/refresh` (cookie present) or, failing that, the Google sign-in.

---

## 9. Security review

- **Device code entropy.** `device_code` = 32 random bytes (`crypto/rand`) base64url (~43 chars), stored **hashed** (SHA-256). It is a bearer secret during the polling window; hashing at rest means a DB read cannot mint tokens.
- **`user_code` format.** 8 chars from an unambiguous alphabet (RFC 8628 recommends excluding easily-confused chars; use Crockford-style `BCDFGHJKLMNPQRSTVWXZ` + digits minus `0/1`), rendered `XXXX-XXXX`. ~20^8 ≈ 2.5e10 space; combined with a 15-min expiry, single-active-code-per-poll, and rate limiting, brute force on the approval page is impractical. Look it up case-insensitively but compare in constant time.
- **Expiry / rotation.** Device codes expire in **15 min**. Access JWT **24h** (fixed). Refresh tokens **30 days**, **rotated on every use**; the old token is revoked immediately, and **reuse of a rotated token triggers family-wide revocation** (compromise signal). Logout revokes.
- **No plaintext secrets at rest.** Neither the refresh token nor the device code is stored in plaintext server-side (only SHA-256 hashes) — the same posture as the stateless cookie storing no server copy. The plaintext refresh token exists only in the CLI's `0600` file.
- **Revocation.** `langner logout` → `/auth/token/revoke`. Access JWTs are stateless and **not** individually revocable before `exp`; the 24h lifetime bounds exposure. If instant kill-switch is needed later, add a `jti`/user denylist checked in the bearer middleware (the `jti` claim is already provisioned).
- **Clock skew.** Verify `exp` with a small leeway (± a few seconds). CLI refreshes proactively at <60s remaining so a slightly-fast client clock never sends a just-expired token.
- **Binding approval to the right user.** The approval page authenticates the browser with a **one-shot Google sign-in** (the authorization-code flow, §8), not an app session cookie; it sets the device's `user_id` only from that freshly server-verified Google identity, so the browser cannot approve a device for someone else. (No persistent app session is created — the Google `state` cookie is short-lived and same-site to the API host.)
- **Enumeration resistance.** `/auth/device/token` returns `expired_token` for both unknown and expired `device_code`; the approval page shows a generic error for bad/consumed `user_code`. Rate-limit `/auth/device/*`.
- **Transport.** All endpoints must be HTTPS in production (Vercel/host-terminated TLS); the CLI should refuse a non-HTTPS `server_url` unless `localhost` (dev).
- **Allowlist applies at sign-in/approval only — NOT at refresh.** `auth.IsAllowed` gates who can complete a Google sign-in (web) or device approval (CLI). But refresh (§4.4) does **not** re-check it, and the no-PII schema (migration 024 stores no email; the token `sub` is the user id) means the server *cannot* re-check the allowlist at refresh without the email. So a de-allowlisted user keeps rolling a 30-day refresh until it expires — roughly parity with today's unrevocable 30-day cookie, but do not overstate it as live enforcement. If allowlist-revocation must be immediate, add a `users.disabled` flag checked in the bearer middleware and at refresh (open question, §10).
- **Device-code phishing / CSRF on approval (RFC 8628 §5.4).** A victim can be social-engineered into approving an attacker's `user_code`. Mitigations: the approval page shows *exactly* what is authorized and requires a deliberate click (do not auto-approve from a `verification_uri_complete`); the `user_code` is bound into the signed `state` (not a forgeable form field), so the approving identity and the code are tamper-locked together through the Google round-trip; short 15-min code expiry + rate-limited `/auth/device/*`.
- **Public-repo secret rules.** `TOKEN_SIGNING_KEY` and `GOOGLE_CLIENT_SECRET` are **env-only**, bound in `config.go`, never written to YAML or committed. (`SESSION_SIGNING_KEY` is removed with the cookie.) `config.example.yml` documents the env var names only. No personal data in examples/tests (use neutral placeholders).

---

## 10. Open questions / decisions still needed

1. **JWT library vs hand-rolled HS256.** Recommend a maintained library (`golang-jwt/jwt/v5`) for `alg` pinning and standards conformance; the repo's minimalist style could justify hand-rolling. **Decide before implementation.**
2. **Exact migration numbers.** This doc assumes 028 is taken by the in-flight `NOT NULL` change → use **029/030**. Re-confirm the next free versions at implementation time.
3. **Web refresh-token storage (§8, M4) — the key security decision.** Proposed default: an **API-origin-scoped `Secure; HttpOnly; SameSite=None` refresh-only cookie** (script-unreadable; XSS bounded to the 24h access token), with access-token delivery by fragment/`postMessage`. Confirm this vs refresh-token-in-memory (no cookie at all; sign in each visit) — it is a genuine tradeoff because that refresh cookie is itself cross-site (though refresh-only). (Signing-key fallback is already decided: **no fallback** — one required `TOKEN_SIGNING_KEY`.)
4. **Device-code cleanup mechanism.** Server goroutine ticker vs `langner-admin` maintenance command vs a DB TTL job — pick one.
5. **`client_id` semantics.** Single fixed `"langner-cli"` public client (no secret, per RFC 8628 for native apps) — confirm we don't want per-build client ids.
6. **Server base URL discovery for the CLI.** How does a non-engineer learn the `--server` value? Options: bake a production default into the binary, ship it in a public `config.example.yml`, or prompt on first `login`. Recommend a compiled-in default (`https://api.langner.app`) overridable by `--server`/env.
7. **`/auth/me` reuse for `whoami`.** Confirm we extend `/auth/me` (already returns username) rather than adding a new endpoint — it already reads the session context that the bearer middleware now fills.
8. **Rate-limiting layer.** Where do `/auth/device/*` rate limits live — app middleware, or rely on the hosting platform (Vercel/edge)?
9. **Two-binary split ordering.** `login`/`logout`/`whoami` land in the user `langner` binary; sequence this against the in-flight split so the commands don't get placed in `langner-admin` by mistake.

---

## 11. Migration / rollout plan

1. **Migrations 029/030** (`cli_device_codes`, `cli_refresh_tokens`) + repositories. Additive, no change to existing tables. Down-migrations drop the new tables only.
2. **Config + key.** Add `TOKEN_SIGNING_KEY` binding (env-only); document in `config.example.yml` (name only). **Remove** the `Auth.Enabled()` gate and `SESSION_SIGNING_KEY`/`SessionSigner`/cookie — auth is always mounted and the server fails fast at startup if `TOKEN_SIGNING_KEY` is unset (§2). This is a breaking change to how the server boots; sequence it so dev (`make dev`) and e2e always provide the key (they already generate a signing key today).
3. **Server endpoints** (`DeviceAuthHandler` + the token-minting web callback, §8) mounted always; **`AuthBearerMiddleware`** replaces the cookie middleware (§6.3); the interceptor is now appended unconditionally and handlers are untouched.
4. **CLI commands** `login`/`logout`/`whoami` + per-server credentials store + auto-refresh interceptor, in the user `langner` binary.
5. **CORS + config.** Add `Authorization` to `Access-Control-Allow-Headers` (`main.go:376` sets only `Content-Type, Connect-Protocol-Version`) and the Vercel frontend origin(s) to `allowed_origins` (fix the pre-existing `localhost:3000` default vs `frontend_url: localhost:3100` mismatch, `config.go:273`). Credentials: pure-bearer RPCs need **no** `Access-Control-Allow-Credentials`; the `/auth/token/*` endpoints DO (they carry the HttpOnly refresh cookie) — scope credentialed CORS to those paths + the exact frontend origin (S5, tied to the M4 refresh-cookie decision).
6. **M1 — local dev / Makefile.** Rewrite `make dev` to always start Postgres and generate `TOKEN_SIGNING_KEY`; remove `AUTH=0`. Document that the server no longer runs DB-less.
7. **M2 — frontend e2e / dev harness migration.** The Playwright harness is cookie-based end to end: `issue-test-cookie` (`cmd/langner/auth.go`) → `seed-and-serve.sh` → `global-setup.ts` builds `storageState` with a literal `langner_session` **cookie**; `reset.ts` re-mints it. Under bearer auth this must become: a new admin **`issue-test-token`** (mints a real access/refresh pair for the seed user) replacing `issue-test-cookie`, and `storageState` seeded via **`origins` localStorage / an injected in-memory token**, not `cookies`. Update `global-setup.ts`, `reset.ts`, `seed-and-serve.sh`. Treat this as part of "done," not deferred.
8. **Example notebooks + real-config integration test** (per repo rule `verify-data-features-with-example-notebooks.md`): construct the server the way `main.go` does from `config.example.yml` (auth always on, DB), drive the full device flow end-to-end against a real Postgres — request a code, approve it via a synthesized approving identity (a test helper that stands in for the Google sign-in and sets the device's `user_id`), poll to success, call a gated RPC with the returned bearer and observe it authenticates, refresh (assert rotation + old-token reuse revokes the family), and revoke (assert subsequent refresh fails). Also assert the always-on gate: an anonymous (no-bearer) RPC still 401s, and a valid bearer authenticates. Add a web-callback test that `/auth/google/callback` mints a token pair (not a `Set-Cookie` session).
9. **Rollback:** the device-flow/CLI pieces are additive (drop the new endpoints/commands, roll 030→029). The cookie→token switch is NOT additive — it is the migration itself; its rollback is reverting to the cookie commit. Sequence the cookie removal as its own reviewable step.
10. **Verification gate (repo rule `no-unverified-main-merges`):** do not propose merge until CI is green on the exact head SHA **and** the flow has been exercised end-to-end against a real Postgres (browser-approval simulated) with the bearer observed authenticating a real RPC, and the web token-mint callback observed returning tokens. Cross-origin behavior that can only be exercised against the live Vercel/Postgres deployment (real SPA↔API CORS, third-party-cookie-free refresh) is called out as unverified until exercised there.
```
