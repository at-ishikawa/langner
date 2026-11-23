package clicmd

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/server"
)

// asUser makes the env act as userID: the quiz calls, the handlers and the
// seeding all run under that account.
func (e *overrideIntervalEnv) asUser(userID int64) {
	e.userID = userID
	e.ctx = auth.WithUserID(context.Background(), userID)
}

// userRows returns the word's logs of quizType written by userID, newest first.
func (e *overrideIntervalEnv) userRows(userID int64, word, quizType string) []overrideIntervalRow {
	var rows []overrideIntervalRow
	require.NoError(e.t, e.db.SelectContext(e.ctx, &rows, `
		SELECT id, status, quality, interval_days, learned_at FROM learning_logs
		WHERE note_id = $1 AND quiz_type = $2 AND user_id = $3 ORDER BY learned_at DESC, id DESC`,
		e.noteID(word), quizType, userID))
	return rows
}

// updateSchedule saves a review schedule through the Settings RPC.
func (e *overrideIntervalEnv) updateSchedule(req *apiv1.UpdateIntervalSettingsRequest) (*apiv1.UpdateIntervalSettingsResponse, error) {
	resp, err := server.NewQuizHandler(e.svc).UpdateIntervalSettings(e.ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// TestUserIntervalSettings_LivePostgres_Integration pins that review
// schedules are per user: the example notebooks from config.example.yml, the
// server's DB-mode wiring, and three accounts answering the same word. A user
// with no schedule gets the server default (the fixed 1,7,30,... ladder); a
// user who saved their own ladder or modified_sm2 on the Settings RPC is
// scheduled with it on answers, on Mark-as-Correct, and by
// `migrate repair-intervals`. Saving a schedule never rewrites past answers.
func TestUserIntervalSettings_LivePostgres_Integration(t *testing.T) {
	env := newOverrideIntervalEnv(t)
	const word, meaning = "pulmonary", "relating to the lungs"
	defaultUser := env.userID
	ladderUser := seedIntegrationUser(t, env.db, "interval-settings-ladder-user")
	sm2User := seedIntegrationUser(t, env.db, "interval-settings-sm2-user")

	// Bring the imported example history in line with the default first, so
	// the repair counts below are only this test's rows.
	_, err := learning.RepairLatestIntervals(env.ctx, env.db, env.repos.Intervals, true)
	require.NoError(t, err)

	t.Run("the Settings RPC saves, validates and reports the schedule", func(t *testing.T) {
		env.t = t
		env.asUser(ladderUser)
		got, err := server.NewQuizHandler(env.svc).GetIntervalSettings(env.ctx, connect.NewRequest(&apiv1.GetIntervalSettingsRequest{}))
		require.NoError(t, err)
		assert.True(t, got.Msg.GetIsDefault(), "a user who saved nothing follows the default")
		assert.Equal(t, "fixed", got.Msg.GetSettings().GetAlgorithm())
		assert.Equal(t, []int32{1, 7, 30, 90, 365, 1095, 1825}, got.Msg.GetSettings().GetFixedIntervals())
		assert.Equal(t, got.Msg.GetSettings(), got.Msg.GetDefaultSettings())

		for _, bad := range [][]int32{{1, 7, 7}, {7, 1}, {0, 3}, {}} {
			_, err := env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{
				Settings: &apiv1.IntervalSettings{Algorithm: "fixed", FixedIntervals: bad},
			})
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "ladder %v must be rejected", bad)
		}
		_, err = env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{
			Settings: &apiv1.IntervalSettings{Algorithm: "weekly"},
		})
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		saved, err := env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{
			Settings: &apiv1.IntervalSettings{Algorithm: "fixed", FixedIntervals: []int32{2, 5, 20, 60}},
		})
		require.NoError(t, err)
		assert.False(t, saved.GetIsDefault())
		assert.Equal(t, []int32{2, 5, 20, 60}, saved.GetSettings().GetFixedIntervals())

		env.asUser(sm2User)
		saved, err = env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{
			Settings: &apiv1.IntervalSettings{Algorithm: "modified_sm2", FixedIntervals: []int32{3, 9}},
		})
		require.NoError(t, err)
		assert.Equal(t, "modified_sm2", saved.GetSettings().GetAlgorithm())
		assert.Empty(t, saved.GetSettings().GetFixedIntervals(), "modified_sm2 keeps no ladder")
	})

	t.Run("the same answer is scheduled with each user's own schedule", func(t *testing.T) {
		env.t = t
		env.reset(word)
		want := map[int64]int{defaultUser: 7, ladderUser: 5, sm2User: 3}
		for userID, days := range want {
			env.asUser(userID)
			for _, qt := range []notebook.QuizType{notebook.QuizTypeNotebook, notebook.QuizTypeReverse} {
				a := env.answer(word, meaning, qt, gradeCorrect, false)
				rows := env.userRows(userID, word, string(qt))
				require.Len(t, rows, 1)
				assert.Equal(t, days, rows[0].IntervalDays, "user %d %s", userID, qt)
				assert.Equal(t, plusDays(t, a.learnedAt, days), a.nextReviewDate, "user %d %s", userID, qt)
			}
		}
	})

	t.Run("Mark-as-Correct uses the user's ladder", func(t *testing.T) {
		env.t = t
		env.reset(word)
		for _, userID := range []int64{defaultUser, ladderUser} {
			env.asUser(userID)
			env.seedCorrect(word, notebook.QuizTypeNotebook) // default → 7, ladder → 5
		}
		env.age(word, 8)

		want := map[int64]struct{ wrong, corrected int }{
			defaultUser: {wrong: 1, corrected: 30},
			ladderUser:  {wrong: 2, corrected: 20},
		}
		for userID, w := range want {
			env.asUser(userID)
			a := env.answer(word, meaning, notebook.QuizTypeNotebook, gradeWrong, false)
			assert.Equal(t, w.wrong, env.userRows(userID, word, "notebook")[0].IntervalDays, "user %d: a wrong answer moves one step down", userID)
			resp := env.override(a, true)
			assert.Equal(t, w.corrected, env.userRows(userID, word, "notebook")[0].IntervalDays, "user %d: corrected answer advances one step", userID)
			assert.Equal(t, plusDays(t, a.learnedAt, w.corrected), resp.GetNextReviewDate())
		}
	})

	t.Run("repair-intervals replays each user with their own schedule", func(t *testing.T) {
		env.t = t
		n, err := learning.RepairLatestIntervals(env.ctx, env.db, env.repos.Intervals, false)
		require.NoError(t, err)
		assert.Zero(t, n, "every series was written with its user's schedule, so nothing needs repair")

		newest := env.userRows(ladderUser, word, "notebook")[0]
		_, err = env.db.ExecContext(env.ctx, `UPDATE learning_logs SET interval_days = 1 WHERE id = $1`, newest.ID)
		require.NoError(t, err)
		n, err = learning.RepairLatestIntervals(env.ctx, env.db, env.repos.Intervals, true)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, 20, env.userRows(ladderUser, word, "notebook")[0].IntervalDays, "replayed with the user's ladder, not the default's 30")
	})

	t.Run("changing or resetting the schedule leaves past answers alone", func(t *testing.T) {
		env.t = t
		env.asUser(ladderUser)
		before := env.userRows(ladderUser, word, "notebook")
		_, err := env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{
			Settings: &apiv1.IntervalSettings{Algorithm: "fixed", FixedIntervals: []int32{3, 10}},
		})
		require.NoError(t, err)
		assert.Equal(t, before, env.userRows(ladderUser, word, "notebook"))

		reset, err := env.updateSchedule(&apiv1.UpdateIntervalSettingsRequest{ResetToDefault: true})
		require.NoError(t, err)
		assert.True(t, reset.GetIsDefault())
		assert.Equal(t, reset.GetDefaultSettings(), reset.GetSettings())
		assert.Equal(t, before, env.userRows(ladderUser, word, "notebook"))

		// After the reset a new answer follows the default again.
		env.reset(word)
		env.answer(word, meaning, notebook.QuizTypeNotebook, gradeCorrect, false)
		assert.Equal(t, 7, env.userRows(ladderUser, word, "notebook")[0].IntervalDays)
	})
}
