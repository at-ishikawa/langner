package datasync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/learning"
	mock_learning "github.com/at-ishikawa/langner/internal/mocks/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestStateSeeder_PersistEtymologyLogs_KeyedByOriginNotNote pins the
// DB-only-state contract for etymology-origin learning logs: the StateSeeder
// writes each of an origin-typed expression's logs against etymology_origins.id
// (OriginID) with quiz_type=etymology_origin, and NEVER attaches them to a
// vocabulary note (NoteID stays zero). This is the write-side half of
// learning-history invariants L1/L4 — one log series per word per mode, keyed
// by origin, so a same-named vocab note cannot end up carrying the origin's
// logs. ImportLearningLogs skips origin-typed expressions for exactly this
// reason (see the "etymology_origin expression is skipped" case in
// datasync_test.go); the seeder is the path that owns them.
//
// The rebase weakened the surrounding assertions; this restores an explicit
// check that two origin logs produce two rows, both keyed by origin.
//
// NOTE: this exercises persistEtymologyLogsForExpression against a mock
// LearningRepository. The true Postgres round-trip (real
// DBLearningRepository.Create → learning_logs.origin_id) is Postgres-only and
// is NOT exercised here (no live DB in this environment).
func TestStateSeeder_PersistEtymologyLogs_KeyedByOriginNotNote(t *testing.T) {
	baseTime := time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)
	learningRepo := mock_learning.NewMockLearningRepository(ctrl)

	const nbID = "demo-roots"
	const originID = int64(100)

	// The origin "alter" is declared for this notebook. A vocabulary note may
	// share the spelling "alter" in the same notebook, but the seeder resolves
	// origin logs strictly through originIDByKey — so they can only land on the
	// origin row.
	originIDByKey := map[etyKey]int64{
		{notebookID: nbID, sessionTitle: "Latin roots", origin: "alter"}: originID,
	}

	expr := notebook.LearningHistoryExpression{
		Expression: "alter",
		Type:       notebook.LearningExpressionTypeOrigin,
		EtymologyOriginLogs: []notebook.LearningRecord{
			{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1200, QuizType: string(notebook.QuizTypeEtymologyOrigin), IntervalDays: 7},
			{Status: "misunderstood", LearnedAt: notebook.NewDate(baseTime.Add(24 * time.Hour)), Quality: 1, ResponseTimeMs: 4200, QuizType: string(notebook.QuizTypeEtymologyOrigin), IntervalDays: 1},
		},
	}

	var created []*learning.LearningLog
	learningRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, log *learning.LearningLog) error {
			// Capture a copy so later mutation of the argument can't affect us.
			cp := *log
			created = append(created, &cp)
			return nil
		})

	seeder := &StateSeeder{learningRepo: learningRepo}
	result := &StateSeedResult{}
	err := seeder.persistEtymologyLogsForExpression(context.Background(), nbID, expr, originIDByKey, result)
	require.NoError(t, err)

	require.Len(t, created, 2, "two etymology-origin logs must produce two rows")
	assert.Equal(t, 2, result.EtymologyLogsCreated)
	for i, log := range created {
		assert.Equalf(t, originID, log.OriginID, "log %d must be keyed by origin_id", i)
		assert.Zerof(t, log.NoteID, "log %d must NOT be attached to a vocab note (no conflation onto a same-named note)", i)
		assert.Equalf(t, string(notebook.QuizTypeEtymologyOrigin), log.QuizType, "log %d must keep quiz_type=etymology_origin", i)
		assert.Equalf(t, nbID, log.SourceNotebookID, "log %d must record its source notebook", i)
	}
	// Status/interval are preserved per record (not collapsed onto one row).
	assert.Equal(t, "understood", created[0].Status)
	assert.Equal(t, 7, created[0].IntervalDays)
	assert.Equal(t, "misunderstood", created[1].Status)
	assert.Equal(t, 1, created[1].IntervalDays)
}

// TestStateSeeder_PersistEtymologyLogs_UnresolvedOriginIsNoOp confirms an
// origin-typed expression whose name resolves to no declared origin writes
// nothing — it is never force-fit onto a note as a fallback.
func TestStateSeeder_PersistEtymologyLogs_UnresolvedOriginIsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningRepo := mock_learning.NewMockLearningRepository(ctrl)
	// No Create expectation: zero calls.

	expr := notebook.LearningHistoryExpression{
		Expression: "unknown-root",
		Type:       notebook.LearningExpressionTypeOrigin,
		EtymologyOriginLogs: []notebook.LearningRecord{
			{Status: "understood", LearnedAt: notebook.NewDate(time.Now()), QuizType: string(notebook.QuizTypeEtymologyOrigin)},
		},
	}

	seeder := &StateSeeder{learningRepo: learningRepo}
	result := &StateSeedResult{}
	err := seeder.persistEtymologyLogsForExpression(context.Background(), "demo-roots", expr, map[etyKey]int64{}, result)
	require.NoError(t, err)
	assert.Zero(t, result.EtymologyLogsCreated)
}
