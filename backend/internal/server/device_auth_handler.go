package server

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/at-ishikawa/langner/internal/auth"
)

const (
	// deviceCodeTTL is how long a device_code / user_code stays valid (RFC 8628
	// recommends a short window; 15 min).
	deviceCodeTTL = 15 * time.Minute
	// devicePollInterval is the minimum seconds a CLI must wait between polls.
	devicePollInterval = 5
	// refreshTokenTTL is the rolling refresh-token lifetime (30 days).
	refreshTokenTTL = 30 * 24 * time.Hour
	// deviceGrantType is the RFC 8628 device_code grant. The single fixed public
	// client id ("langner-cli", RFC 8628 native app) is accepted leniently — it
	// carries no secret, so it is not validated.
	deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

// DeviceAuthHandler serves the RFC 8628 device-authorization endpoints and the
// refresh/revoke endpoints for the CLI bearer-token flow. They are plain HTTP,
// mounted OUTSIDE the connect interceptor so an unauthenticated CLI can reach
// them. Approval reuses the EXISTING langner_session cookie sign-in (this
// additive PR); the approve-inside-OAuth-callback form is a later PR.
type DeviceAuthHandler struct {
	deviceCodes   *auth.CLIDeviceCodeRepository
	refreshTokens *auth.CLIRefreshTokenRepository
	tokens        *auth.TokenSigner
	frontendURL   string
	// verificationURI is the API-host URL the user opens to approve, e.g.
	// https://api.langner.app/auth/device.
	verificationURI string
	limiter         *rateLimiter
	now             func() time.Time
}

// DeviceAuthHandlerConfig wires a DeviceAuthHandler.
type DeviceAuthHandlerConfig struct {
	DeviceCodes     *auth.CLIDeviceCodeRepository
	RefreshTokens   *auth.CLIRefreshTokenRepository
	Tokens          *auth.TokenSigner
	FrontendURL     string
	VerificationURI string
	// Now is optional; defaults to time.Now. Tests pin it.
	Now func() time.Time
}

// NewDeviceAuthHandler builds a DeviceAuthHandler.
func NewDeviceAuthHandler(cfg DeviceAuthHandlerConfig) *DeviceAuthHandler {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &DeviceAuthHandler{
		deviceCodes:     cfg.DeviceCodes,
		refreshTokens:   cfg.RefreshTokens,
		tokens:          cfg.Tokens,
		frontendURL:     cfg.FrontendURL,
		verificationURI: cfg.VerificationURI,
		limiter:         newRateLimiter(20, time.Minute),
		now:             now,
	}
}

// --- request/response shapes ---

type deviceCodeRequest struct {
	ClientID string `json:"client_id"`
}

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenRequest struct {
	GrantType  string `json:"grant_type"`
	DeviceCode string `json:"device_code"`
	ClientID   string `json:"client_id"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type revokeRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RequestCode handles POST /auth/device/code (RFC 8628 §3.1/§3.2).
func (h *DeviceAuthHandler) RequestCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	if !h.limiter.allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "slow_down")
		return
	}
	var req deviceCodeRequest
	// Body is optional/lenient; client_id is a public fixed id.
	_ = json.NewDecoder(r.Body).Decode(&req)

	deviceCode, err := auth.GenerateDeviceCode()
	if err != nil {
		slog.Error("device: generate device code failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	userCode, err := auth.GenerateUserCode()
	if err != nil {
		slog.Error("device: generate user code failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	expiresAt := h.now().Add(deviceCodeTTL)
	if _, err := h.deviceCodes.Create(r.Context(), auth.HashSecret(deviceCode), userCode, expiresAt); err != nil {
		slog.Error("device: create device code failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}

	writeJSON(w, http.StatusOK, deviceCodeResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         h.verificationURI,
		VerificationURIComplete: h.verificationURI + "?user_code=" + userCode,
		ExpiresIn:               int(deviceCodeTTL.Seconds()),
		Interval:                devicePollInterval,
	})
}

var devicePageTmpl = template.Must(template.New("device").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Approve CLI access</title></head>
<body style="font-family:sans-serif;max-width:32rem;margin:4rem auto;">
{{if .Signed}}
  <h1>Approve CLI access?</h1>
  <p>Enter the code shown in your terminal by <code>langner login</code>, then Approve.
     Only approve a code you started yourself.</p>
  <form method="post">
    <input type="text" name="user_code" value="{{.UserCode}}" placeholder="XXXX-XXXX"
      autocomplete="off" autocapitalize="characters" spellcheck="false" required
      style="font-size:1.25rem;letter-spacing:0.15em;text-transform:uppercase;padding:0.4rem 0.6rem;width:9rem;">
    <div style="margin-top:1rem;">
      <button type="submit" formaction="/auth/device/approve">Approve</button>
      <button type="submit" formaction="/auth/device/deny" style="margin-left:1rem;">Deny</button>
    </div>
  </form>
{{else}}
  <h1>Sign in to approve CLI access</h1>
  <p>Sign in, then enter the code shown in your terminal by <code>langner login</code>.</p>
  <p><a href="/auth/google/login">Sign in with Google</a></p>
{{end}}
</body></html>`))

// ShowApprovalPage handles GET /auth/device: it renders the confirmation page.
// The user must be signed in (langner_session cookie, lifted into the request
// context by the cookie middleware) to see the Approve/Deny controls.
func (h *DeviceAuthHandler) ShowApprovalPage(w http.ResponseWriter, r *http.Request) {
	userCode := auth.NormalizeUserCode(r.URL.Query().Get("user_code"))
	_, signedIn := auth.SessionFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = devicePageTmpl.Execute(w, struct {
		Signed   bool
		UserCode string
	}{Signed: signedIn, UserCode: userCode})
}

// Approve handles POST /auth/device/approve. It binds the signed-in user
// (from the session cookie) to the device row for the submitted user_code.
func (h *DeviceAuthHandler) Approve(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/auth/device?user_code="+auth.NormalizeUserCode(r.FormValue("user_code")), http.StatusFound)
		return
	}
	userCode := auth.NormalizeUserCode(r.FormValue("user_code"))
	if !h.limiter.allow(clientIP(r)) {
		h.writeApprovalResult(w, "Too many attempts. Please wait and try again.")
		return
	}
	row, err := h.deviceCodes.FindByUserCode(r.Context(), userCode)
	if err != nil || row.Status != auth.DeviceStatusPending || !h.now().Before(row.ExpiresAt) {
		// Generic message — no enumeration signal for bad/expired/consumed codes.
		h.writeApprovalResult(w, "That code is invalid or has expired. Return to your terminal and try again.")
		return
	}
	if err := h.deviceCodes.Approve(r.Context(), row.ID, sess.UserID, h.now()); err != nil {
		h.writeApprovalResult(w, "That code is invalid or has expired. Return to your terminal and try again.")
		return
	}
	h.writeApprovalResult(w, "Approved. You can return to your terminal.")
}

// Deny handles POST /auth/device/deny. No identity is needed to deny.
func (h *DeviceAuthHandler) Deny(w http.ResponseWriter, r *http.Request) {
	userCode := auth.NormalizeUserCode(r.FormValue("user_code"))
	_ = h.deviceCodes.Deny(r.Context(), userCode)
	h.writeApprovalResult(w, "Denied. You can return to your terminal.")
}

func (h *DeviceAuthHandler) writeApprovalResult(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><body style="font-family:sans-serif;max-width:32rem;margin:4rem auto;"><p>` +
		template.HTMLEscapeString(msg) + `</p></body></html>`))
}

// Token handles POST /auth/device/token (RFC 8628 §3.4/§3.5): the CLI poll.
func (h *DeviceAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	var req deviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.GrantType != deviceGrantType {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if req.DeviceCode == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	row, err := h.deviceCodes.FindByDeviceCodeHash(r.Context(), auth.HashSecret(req.DeviceCode))
	if err != nil {
		// Unknown == expired: do not distinguish (enumeration resistance).
		writeOAuthError(w, http.StatusBadRequest, "expired_token")
		return
	}
	now := h.now()
	if !now.Before(row.ExpiresAt) {
		writeOAuthError(w, http.StatusBadRequest, "expired_token")
		return
	}
	// slow_down: reject a poll that arrives faster than the interval.
	if row.LastPolledAt.Valid && now.Sub(row.LastPolledAt.Time) < devicePollInterval*time.Second {
		_ = h.deviceCodes.MarkPolled(r.Context(), row.ID, now)
		writeOAuthError(w, http.StatusBadRequest, "slow_down")
		return
	}
	_ = h.deviceCodes.MarkPolled(r.Context(), row.ID, now)

	switch row.Status {
	case auth.DeviceStatusDenied:
		writeOAuthError(w, http.StatusBadRequest, "access_denied")
		return
	case auth.DeviceStatusConsumed:
		// Already spent — treat as expired (no second mint).
		writeOAuthError(w, http.StatusBadRequest, "expired_token")
		return
	case auth.DeviceStatusApproved:
		// fall through to mint
	default: // pending
		writeOAuthError(w, http.StatusBadRequest, "authorization_pending")
		return
	}
	if !row.UserID.Valid {
		writeOAuthError(w, http.StatusBadRequest, "authorization_pending")
		return
	}

	// Consume first (idempotency guard against a concurrent double-mint), then
	// issue tokens.
	if err := h.deviceCodes.Consume(r.Context(), row.ID); err != nil {
		// Lost the race / already consumed.
		writeOAuthError(w, http.StatusBadRequest, "expired_token")
		return
	}
	h.issueTokens(w, r, row.UserID.Int64)
}

// Refresh handles POST /auth/token/refresh (rotation-on-use).
func (h *DeviceAuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	row, err := h.refreshTokens.FindByHash(r.Context(), auth.HashSecret(req.RefreshToken))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	now := h.now()
	// Reuse of an already-revoked (rotated-away) token is a compromise signal:
	// revoke the whole family and reject.
	if row.RevokedAt.Valid {
		if err := h.refreshTokens.RevokeFamily(r.Context(), row.UserID, now); err != nil {
			slog.Error("device: revoke family failed", "error", err)
		}
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	if !now.Before(row.ExpiresAt) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	// Rotate: revoke the presented token, mint a fresh pair.
	if err := h.refreshTokens.Revoke(r.Context(), row.ID, now); err != nil {
		slog.Error("device: revoke on rotation failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	h.issueTokens(w, r, row.UserID)
}

// Revoke handles POST /auth/token/revoke (logout). Always 200 (idempotent).
func (h *DeviceAuthHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	var req revokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
		if err := h.refreshTokens.RevokeByHash(r.Context(), auth.HashSecret(req.RefreshToken), h.now()); err != nil {
			slog.Error("device: revoke failed", "error", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

// issueTokens mints an access JWT + a fresh refresh token (stored hashed) and
// writes the success response.
func (h *DeviceAuthHandler) issueTokens(w http.ResponseWriter, r *http.Request, userID int64) {
	now := h.now()
	access, err := h.tokens.Sign(userID, now)
	if err != nil {
		slog.Error("device: sign access token failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	refresh, err := auth.GenerateRefreshToken()
	if err != nil {
		slog.Error("device: generate refresh token failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	if _, err := h.refreshTokens.Create(r.Context(), userID, auth.HashSecret(refresh), now.Add(refreshTokenTTL)); err != nil {
		slog.Error("device: store refresh token failed", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
		RefreshToken: refresh,
	})
}

// --- helpers ---

type oauthError struct {
	Error string `json:"error"`
}

func writeOAuthError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, oauthError{Error: code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
