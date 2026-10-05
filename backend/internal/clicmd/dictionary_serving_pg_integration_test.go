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
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// TestServeDictionaryFromDB_LivePostgres_Integration reproduces and pins the
// production failure where the vocabulary quiz 500'd with
// "definition.SetDetails() > there is no meaning, images, nor statements for
// word" for a story word whose meaning comes from the dictionary. On a deployed
// server the on-disk rapidapi cache is absent, so dictionaryMap was empty and
// SetDetails could not fill the word's meaning. The dictionary lives in
// dictionary_entries, so serving must build the map from the DB.
//
// This drives the EXACT failing call (Note.SetDetails) with the dictionary
// loaded from Postgres via DBDictionaryRepository.LoadResponseMap — the same
// construction the server now uses — and asserts it resolves, while an empty map
// still fails (the prod symptom). Generic word, no personal data.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs/recreates the public schema, so it must run isolated (-p 1).
func TestServeDictionaryFromDB_LivePostgres_Integration(t *testing.T) {
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
	const meaning = "lasting a very short time"
	response, err := json.Marshal(rapidapi.Response{
		Word:          "ephemeral",
		Pronunciation: rapidapi.Pronunciation{All: "ih-fem-er-uhl"},
		Results:       []rapidapi.Result{{Definition: meaning, PartOfSpeech: "adjective"}},
	})
	require.NoError(t, err)

	repo := dictionary.NewDBDictionaryRepository(db)
	require.NoError(t, repo.BatchUpsert(ctx, []*dictionary.DictionaryEntry{
		{Word: "ephemeral", SourceType: "rapidapi", Response: response},
	}))

	// The map the server builds for serving, loaded from Postgres (not the
	// filesystem cache).
	dbMap, err := repo.LoadResponseMap(ctx)
	require.NoError(t, err)
	require.Contains(t, dbMap, "ephemeral")
	require.NotEmpty(t, dbMap["ephemeral"].Results)

	// A story word whose meaning comes from the dictionary (dictionary_number set,
	// no inline meaning), exactly like the word that broke production.
	newWord := func() *notebook.Note {
		return &notebook.Note{
			Expression:       "Ephemeral",
			Definition:       "ephemeral",
			Level:            notebook.ExpressionLevelNew,
			DictionaryNumber: 1,
		}
	}

	// Empty map (the prod bug: no filesystem cache, DB not consulted) → fails.
	require.Error(t, newWord().SetDetails(map[string]rapidapi.Response{}, ""),
		"an empty dictionary map must still reproduce the production failure")

	// DB-backed map → resolves the meaning, no error.
	note := newWord()
	require.NoError(t, note.SetDetails(dbMap, ""),
		"the DB-loaded dictionary must let SetDetails fill the word's meaning")
	require.Equal(t, meaning, note.Meaning)
}
