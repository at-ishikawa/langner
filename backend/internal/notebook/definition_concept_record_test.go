package notebook

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDBDefinitionConceptRepoForTest(t *testing.T) (*DBDefinitionConceptRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(mockDB, "pgx")
	return NewDBDefinitionConceptRepository(sqlxDB), mock, func() { _ = mockDB.Close() }
}

func TestDBDefinitionConceptRepository_ListDefinitionConceptsByNotebook(t *testing.T) {
	repo, mock, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "notebook_id", "head", "meaning", "created_at", "updated_at"}).
		AddRow(1, "wpme", "ambidextrous", "skilled with both hands", now, now).
		AddRow(2, "wpme", "ambivalent", "having mixed feelings", now, now)
	mock.ExpectQuery(`SELECT id, notebook_id, head, meaning, created_at, updated_at FROM definition_concepts WHERE notebook_id = \$1`).
		WithArgs("wpme").
		WillReturnRows(rows)

	got, err := repo.ListDefinitionConceptsByNotebook(context.Background(), "wpme")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "ambidextrous", got[0].Head)
	assert.Equal(t, "skilled with both hands", got[0].Meaning)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBDefinitionConceptRepository_ListAllDefinitionConcepts(t *testing.T) {
	repo, mock, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "notebook_id", "head", "meaning", "created_at", "updated_at"}).
		AddRow(1, "book-a", "bright", "having or emitting much light", now, now).
		AddRow(2, "book-b", "dim", "lacking light", now, now)
	mock.ExpectQuery("SELECT id, notebook_id, head, meaning, created_at, updated_at FROM definition_concepts").
		WillReturnRows(rows)

	got, err := repo.ListAllDefinitionConcepts(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "book-a", got[0].NotebookID)
	assert.Equal(t, "book-b", got[1].NotebookID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBDefinitionConceptRepository_ListDefinitionConceptMembersByConceptIDs_Empty(t *testing.T) {
	repo, _, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	got, err := repo.ListDefinitionConceptMembersByConceptIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDBDefinitionConceptRepository_ListDefinitionConceptMembersByConceptIDs(t *testing.T) {
	repo, mock, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "concept_id", "expression", "session_title", "created_at"}).
		AddRow(10, 1, "bright", "Chapter 1", now).
		AddRow(11, 1, "brighten", "Chapter 1", now)
	mock.ExpectQuery("SELECT id, concept_id, expression, session_title, created_at FROM definition_concept_members WHERE concept_id IN").
		WithArgs(int64(1), int64(2)).
		WillReturnRows(rows)

	got, err := repo.ListDefinitionConceptMembersByConceptIDs(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "bright", got[0].Expression)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBDefinitionConceptRepository_BatchCreateConcepts_Empty(t *testing.T) {
	repo, _, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	require.NoError(t, repo.BatchCreateConcepts(context.Background(), nil))
}

func TestDBDefinitionConceptRepository_BatchDeleteConcepts_Empty(t *testing.T) {
	repo, _, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	require.NoError(t, repo.BatchDeleteConcepts(context.Background(), nil))
}

func TestDBDefinitionConceptRepository_BatchCreateMembers_Empty(t *testing.T) {
	repo, _, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	require.NoError(t, repo.BatchCreateMembers(context.Background(), nil))
}

func TestDBDefinitionConceptRepository_BatchDeleteMembers_Empty(t *testing.T) {
	repo, _, cleanup := newDBDefinitionConceptRepoForTest(t)
	defer cleanup()

	require.NoError(t, repo.BatchDeleteMembers(context.Background(), nil))
}

func TestDefinitionConceptKey(t *testing.T) {
	// Stable composite key for dedup; the importer relies on this layout.
	assert.Equal(t, "book-a\x00head", definitionConceptKey("book-a", "head"))
	assert.NotEqual(t, definitionConceptKey("ab", "c"), definitionConceptKey("a", "bc"))
}
