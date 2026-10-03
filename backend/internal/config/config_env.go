package config

import (
	"os"
	"strconv"
	"strings"
)

// applyEnvOverrides lets EVERY deploy-relevant setting be provided by an
// environment variable, so the server can run with NO config.yml on disk — the
// shape a serverless/PaaS deploy (e.g. Vercel functions) needs, where mounting a
// file is awkward. It runs AFTER the YAML load, so env wins over file, and
// BEFORE validation, so env-provided values are validated too.
//
// Secrets (DB_PASSWORD, GOOGLE_CLIENT_SECRET, TOKEN_SIGNING_KEY, OPENAI/GEMINI/
// RAPID keys, INFERENCE_MODE) are already bound by viper in Load and are not
// repeated here. These are the NON-secret settings that previously lived only in
// the YAML file: server, database (incl. the pooler knobs), CORS, and the auth
// URLs/flags. Names use the DB_ prefix for database fields to match the existing
// DB_PASSWORD, and PORT (the PaaS convention) overrides server.port.
func applyEnvOverrides(cfg *Config) {
	// Server. PORT is the platform-injected convention; SERVER_PORT is explicit.
	// Whichever is set last wins, so PORT (checked last) takes precedence.
	envInt(&cfg.Server.Port, "SERVER_PORT")
	envInt(&cfg.Server.Port, "PORT")
	envCSV(&cfg.Server.CORS.AllowedOrigins, "CORS_ALLOWED_ORIGINS")

	// Database — host/port/name/user/tls plus the generic pooler knobs. Point
	// host/port at ANY transaction pooler (PgBouncer, Supabase's pooler, pgcat);
	// DB_PARAMS ("k=v,k=v") passes through arbitrary DSN params (sslmode,
	// default_query_exec_mode, …) for pooler-specific tuning without code changes.
	envStr(&cfg.Database.Host, "DB_HOST")
	envInt(&cfg.Database.Port, "DB_PORT")
	envStr(&cfg.Database.Database, "DB_NAME")
	envStr(&cfg.Database.Username, "DB_USER")
	envBool(&cfg.Database.TLS, "DB_TLS")
	envInt(&cfg.Database.MaxOpenConns, "DB_MAX_OPEN_CONNS")
	envInt(&cfg.Database.MaxIdleConns, "DB_MAX_IDLE_CONNS")
	envInt(&cfg.Database.ConnMaxLifetime, "DB_CONN_MAX_LIFETIME_SECONDS")
	envKV(&cfg.Database.Params, "DB_PARAMS")

	// Auth (non-secret). The redirect/frontend URLs and cookie flags all move to
	// prod values; allowed_emails is the sign-in allowlist.
	envStr(&cfg.Auth.GoogleClientID, "AUTH_GOOGLE_CLIENT_ID")
	envStr(&cfg.Auth.RedirectURL, "AUTH_REDIRECT_URL")
	envStr(&cfg.Auth.FrontendURL, "AUTH_FRONTEND_URL")
	envCSV(&cfg.Auth.AllowedEmails, "AUTH_ALLOWED_EMAILS")
	envStr(&cfg.Auth.InitialAdminEmail, "AUTH_INITIAL_ADMIN_EMAIL")
	envBool(&cfg.Auth.CookieSecure, "AUTH_COOKIE_SECURE")
	envStr(&cfg.Auth.CookieSameSite, "AUTH_COOKIE_SAMESITE")
}

// envStr sets *dst to the env value when the variable is present (even if empty,
// so it can be deliberately cleared). Absent variable leaves *dst untouched.
func envStr(dst *string, key string) {
	if v, ok := os.LookupEnv(key); ok {
		*dst = v
	}
}

// envInt sets *dst only when the variable is present AND parses as an int, so a
// malformed value is ignored rather than silently zeroing a good default.
func envInt(dst *int, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			*dst = n
		}
	}
}

// envBool sets *dst when the variable parses as a bool (1/t/true/0/f/false).
func envBool(dst *bool, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			*dst = b
		}
	}
}

// envCSV sets *dst to the comma-separated env value (trimmed, empties dropped);
// an empty value clears the list. Absent variable leaves *dst untouched.
func envCSV(dst *[]string, key string) {
	if v, ok := os.LookupEnv(key); ok {
		*dst = splitList(v)
	}
}

// envKV merges a "k=v,k=v" env value into *dst (creating the map if nil), so
// DSN params can be set/extended from the environment.
func envKV(dst *map[string]string, key string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	for _, pair := range splitList(v) {
		k, val, found := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !found || k == "" {
			continue
		}
		if *dst == nil {
			*dst = map[string]string{}
		}
		(*dst)[k] = strings.TrimSpace(val)
	}
}

// splitList splits a comma-separated list, trimming each element and dropping
// empties.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
