package clicmd

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/schemas"
)

// TestDBDictionaryReader_LookupHit_LivePostgres_Integration pins the DB-backed
// live lookup: a word already in dictionary_entries resolves from Postgres with
// NO filesystem cache and NO external API call. (The miss path — fetch from the
// RapidAPI service then persist — can't be exercised without live API
// credentials and is not covered here.)
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs/recreates the public schema, so it must run isolated (-p 1).
func TestDBDictionaryReader_LookupHit_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	ctx := context.Background()
	response, err := json.Marshal(rapidapi.Response{
		Word:    "ephemeral",
		Results: []rapidapi.Result{{Definition: "lasting a very short time", PartOfSpeech: "adjective"}},
	})
	require.NoError(t, err)

	repo := dictionary.NewDBDictionaryRepository(db)
	require.NoError(t, repo.BatchUpsert(ctx, []*dictionary.DictionaryEntry{
		{Word: "ephemeral", SourceType: "rapidapi", Response: response},
	}))

	// No API key/host configured: a hit must be served from the DB without any
	// external call or filesystem access.
	reader := dictionary.NewDBReader(repo, dictionary.Config{})
	resp, err := reader.Lookup(ctx, "ephemeral")
	require.NoError(t, err)
	require.Equal(t, "ephemeral", resp.Word)
	require.NotEmpty(t, resp.Results)
	require.Equal(t, "lasting a very short time", resp.Results[0].Definition)
}
