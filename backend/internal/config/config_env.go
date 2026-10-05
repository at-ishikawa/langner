package config

import (
	"os"
	"strconv"
	"strings"
)

// nonSecretEnvBindings maps each non-secret config key to the environment
// variable(s) that may override it, so the server can run with NO config.yml
// (serverless/PaaS). Explicit BindEnv — NOT AutomaticEnv — because AutomaticEnv
// does not register nested, default-less keys for Unmarshal; binding each key by
// name does, and pins a stable, documented env name. viper precedence makes env
// win over the file. `server.port` accepts PORT (the PaaS convention) first,
// then SERVER_PORT. (database.params is a map BindEnv can't express — DB_PARAMS
// is merged in applyEnvExtras.)
var nonSecretEnvBindings = [][]string{
	{"server.port", "PORT", "SERVER_PORT"},
	{"server.cors.allowed_origins", "CORS_ALLOWED_ORIGINS"},
	{"database.host", "DB_HOST"},
	{"database.port", "DB_PORT"},
	{"database.database", "DB_NAME"},
	{"database.username", "DB_USER"},
	{"database.tls", "DB_TLS"},
	{"database.max_open_conns", "DB_MAX_OPEN_CONNS"},
	{"database.max_idle_conns", "DB_MAX_IDLE_CONNS"},
	{"database.conn_max_lifetime_seconds", "DB_CONN_MAX_LIFETIME_SECONDS"},
	{"auth.google_client_id", "AUTH_GOOGLE_CLIENT_ID"},
	{"auth.redirect_url", "AUTH_REDIRECT_URL"},
	{"auth.frontend_url", "AUTH_FRONTEND_URL"},
	{"auth.allowed_emails", "AUTH_ALLOWED_EMAILS"},
	{"auth.initial_admin_email", "AUTH_INITIAL_ADMIN_EMAIL"},
	{"auth.cookie_secure", "AUTH_COOKIE_SECURE"},
	{"auth.cookie_samesite", "AUTH_COOKIE_SAMESITE"},
	{"quiz.algorithm", "QUIZ_ALGORITHM"},
	// quiz.fixed_intervals ([]int) is not bound here — a comma-separated env
	// value needs custom int parsing (see applyEnvExtras / QUIZ_FIXED_INTERVALS).
}

// applyEnvExtras handles the two things viper's BindEnv can't do cleanly, run
// after Unmarshal: (1) merge DB_PARAMS ("k=v,k=v") into the database.params map,
// and (2) trim whitespace from comma-separated list values (viper's
// StringToSlice hook splits on "," but does not trim), so `A, B` yields
// ["A","B"] not ["A"," B"].
func applyEnvExtras(cfg *Config) {
	mergeKVInto(&cfg.Database.Params, os.Getenv("DB_PARAMS"))
	cfg.Server.CORS.AllowedOrigins = trimList(cfg.Server.CORS.AllowedOrigins)
	cfg.Auth.AllowedEmails = trimList(cfg.Auth.AllowedEmails)
	// QUIZ_FIXED_INTERVALS overrides quiz.fixed_intervals ([]int) from a
	// comma-separated env value, e.g. "1,7,30,90". Parsed here (not via BindEnv)
	// so spaces are tolerated and bad entries skipped.
	if ints, ok := intsFromCSV(os.Getenv("QUIZ_FIXED_INTERVALS")); ok {
		cfg.Quiz.FixedIntervals = ints
	}
}

// intsFromCSV parses a comma-separated int list ("1, 7, 30"), trimming each and
// skipping non-numeric entries. Returns ok=false when s has no valid int, so the
// caller keeps the existing (file/default) value.
func intsFromCSV(s string) ([]int, bool) {
	var out []int
	for _, p := range splitList(s) {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out, len(out) > 0
}

// mergeKVInto parses a "k=v,k=v" string and merges it into *dst (creating the
// map if nil). Empty string is a no-op; a pair with no '=' or empty key is
// skipped.
func mergeKVInto(dst *map[string]string, s string) {
	for _, pair := range splitList(s) {
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

// trimList trims each element and drops empties, normalizing a slice whether it
// came from the file or from a comma-split env value.
func trimList(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// splitList splits a comma-separated list, trimming and dropping empties.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
