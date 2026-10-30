package database

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
)

// openQueryStatsTestDB opens LANGNER_INTEGRATION_DB_URL through Open (so the
// byte-counting dialer and tracer are installed) with a small table of rows
// holding a distinctive value that must never reach the logs.
func openQueryStatsTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	password, _ := u.User.Password()
	db, err := Open(config.DatabaseConfig{
		Host:     u.Hostname(),
		Port:     port,
		Database: strings.TrimPrefix(u.Path, "/"),
		Username: u.User.Username(),
		Password: password,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`DROP TABLE IF EXISTS query_stats_test_items;
		CREATE TABLE query_stats_test_items (id int, body text);
		INSERT INTO query_stats_test_items SELECT g, 'private-row-value-' || g FROM generate_series(1, 50) g`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS query_stats_test_items`) })
	return db
}

// captureLogs routes slog to a JSON buffer for the duration of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func logLines(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &line))
		if line["msg"] == msg {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestQueryStatsMiddleware_LivePostgres_Integration(t *testing.T) {
	db := openQueryStatsTestDB(t)

	tests := []struct {
		name         string
		ctx          func(r *http.Request) context.Context
		wantInferred float64
	}{
		{
			name:         "query with the request context",
			ctx:          func(r *http.Request) context.Context { return r.Context() },
			wantInferred: 0,
		},
		{
			name:         "query without the request context is attributed to the sole in-flight request",
			ctx:          func(*http.Request) context.Context { return context.Background() },
			wantInferred: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)
			handler := QueryStatsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := tt.ctx(r)
				// The same query twice (an N+1 shape) plus a different one.
				for range 2 {
					var bodies []string
					require.NoError(t, db.SelectContext(ctx, &bodies, `SELECT body FROM query_stats_test_items WHERE id <= $1`, 50))
					require.Len(t, bodies, 50)
				}
				var count int
				require.NoError(t, db.GetContext(ctx, &count, `SELECT count(*) FROM query_stats_test_items`))
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api.v1.QuizService/StartQuiz", nil))

			assert.NotContains(t, logs.String(), "private-row-value", "result data must never be logged")
			lines := logLines(t, logs, "db usage")
			require.Len(t, lines, 1, logs.String())
			line := lines[0]
			assert.Equal(t, "/api.v1.QuizService/StartQuiz", line["endpoint"])
			assert.Equal(t, float64(3), line["queries"])
			assert.Equal(t, float64(101), line["rows"])
			assert.Equal(t, tt.wantInferred, line["inferred_queries"])
			// 100 rows of ~20-byte bodies plus protocol overhead.
			assert.Greater(t, line["bytes_in"], float64(2000))

			top := line["top"].([]any)
			require.Len(t, top, 2)
			heaviest := top[0].(string)
			assert.Contains(t, heaviest, `sql="SELECT body FROM query_stats_test_items WHERE id <= $1"`)
			assert.Contains(t, heaviest, " calls=2 rows=100 ")
		})
	}
}

func TestQueryStatsMiddleware_NoQueries_NoLine_LivePostgres_Integration(t *testing.T) {
	openQueryStatsTestDB(t)
	logs := captureLogs(t)
	handler := QueryStatsMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	assert.Empty(t, logLines(t, logs, "db usage"))
}

func TestQueryTracer_OutsideAnyRequest_LivePostgres_Integration(t *testing.T) {
	db := openQueryStatsTestDB(t)
	query := func() {
		var bodies []string
		require.NoError(t, db.SelectContext(context.Background(), &bodies, `SELECT body FROM query_stats_test_items`))
	}

	t.Run("silent by default (CLI tools)", func(t *testing.T) {
		logs := captureLogs(t)
		query()
		assert.Empty(t, logLines(t, logs, "db usage (unattributed query)"))
	})

	t.Run("logged once the server enables it", func(t *testing.T) {
		LogUnattributedQueries()
		t.Cleanup(func() { logUnattributed.Store(false) })
		logs := captureLogs(t)
		query()

		assert.NotContains(t, logs.String(), "private-row-value")
		lines := logLines(t, logs, "db usage (unattributed query)")
		require.Len(t, lines, 1, logs.String())
		assert.Equal(t, "SELECT body FROM query_stats_test_items", lines[0]["sql"])
		assert.Equal(t, float64(50), lines[0]["rows"])
		assert.Greater(t, lines[0]["bytes_in"], float64(1000))
	})
}
