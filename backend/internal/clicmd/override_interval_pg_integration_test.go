package clicmd

import (
	"context"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/server"
	"github.com/at-ishikawa/langner/schemas"
)

// overrideIntervalEnv is a live-Postgres harness wired the way langner-server
// runs in DB mode: the example notebooks from config.example.yml imported with
// the real importer, DB-only state repositories, the fixed interval ladder and
// QuizHandlers whose grader is a mock (so each answer's grade is chosen by the
// test). History is seeded through the real write path (Service.Save*Result)
// and aged by moving learned_at back, like the other interval tests.
type overrideIntervalEnv struct {
	t      *testing.T
	ctx    context.Context
	db     *sqlx.DB
	repos  bootstrap.StateRepositories
	svc    *quiz.Service
	userID int64
	// grade is what the mock grader returns for the next answer.
	grade inference.ValidateWordFormResponse
}

const overrideIntervalBook = "reverse-progress-demo"

var overrideIntervalLadder = []int{1, 7, 30, 90, 365, 1095, 1825}

func newOverrideIntervalEnv(t *testing.T) *overrideIntervalEnv {
	t.Helper()
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)
	importOwner := seedIntegrationUser(t, db, "override-interval-import-owner")
	_, err = newImporterFromConfig(cfg, db, io.Discard).ImportAll(context.Background(), datasync.ImportOptions{OwnerID: importOwner})
	require.NoError(t, err)

	env := &overrideIntervalEnv{t: t, db: db}
	ctrl := gomock.NewController(t)
	client := mock_inference.NewMockClient(ctrl)
	client.EXPECT().ValidateWordFormBatch(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
			out := make([]inference.ValidateWordFormResponse, len(params))
			for i := range params {
				out[i] = env.grade
				out[i].Index = i
			}
			return out, nil
		}).AnyTimes()
	client.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			correct := env.grade.Classification == inference.ClassificationSameWord
			return inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{
				Expression:        req.Expressions[0].Expression,
				Meaning:           req.Expressions[0].Meaning,
				AnswersForContext: []inference.AnswersForContext{{Correct: correct, Reason: env.grade.Reason, Quality: env.grade.Quality}},
			}}}, nil
		}).AnyTimes()

	env.repos = bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, env.repos.HistoryStore)
	env.svc = quiz.NewService(cfg.Notebooks, inference.StaticResolver(client), nil, env.repos.Learning,
		config.QuizConfig{Algorithm: "fixed", FixedIntervals: overrideIntervalLadder})
	env.svc.SetHistoryStore(env.repos.HistoryStore)
	env.userID = seedIntegrationUser(t, db, "override-interval-user")
	env.ctx = auth.WithUserID(context.Background(), env.userID)
	return env
}

// noteID returns the imported note of the example word.
func (e *overrideIntervalEnv) noteID(word string) int64 {
	var id int64
	require.NoError(e.t, e.db.GetContext(e.ctx, &id, `SELECT id FROM notes WHERE "usage" = $1`, word))
	return id
}

// reset deletes every log of the word so a scenario starts from no history.
func (e *overrideIntervalEnv) reset(word string) {
	_, err := e.db.ExecContext(e.ctx, `DELETE FROM learning_logs WHERE note_id = $1`, e.noteID(word))
	require.NoError(e.t, err)
}

// age moves every log of the word back by days, as if they were answered
// that much earlier.
func (e *overrideIntervalEnv) age(word string, days int) {
	_, err := e.db.ExecContext(e.ctx,
		`UPDATE learning_logs SET learned_at = learned_at - make_interval(days => $1) WHERE note_id = $2`,
		days, e.noteID(word))
	require.NoError(e.t, err)
}

type overrideIntervalRow struct {
	ID           int64     `db:"id"`
	Status       string    `db:"status"`
	Quality      int       `db:"quality"`
	IntervalDays int       `db:"interval_days"`
	LearnedAt    time.Time `db:"learned_at"`
}

// rows returns the word's logs of quizType, newest first.
func (e *overrideIntervalEnv) rows(word, quizType string) []overrideIntervalRow {
	var rows []overrideIntervalRow
	require.NoError(e.t, e.db.SelectContext(e.ctx, &rows, `
		SELECT id, status, quality, interval_days, learned_at FROM learning_logs
		WHERE note_id = $1 AND quiz_type = $2 ORDER BY learned_at DESC, id DESC`,
		e.noteID(word), quizType))
	return rows
}

func (e *overrideIntervalEnv) latest(word, quizType string) overrideIntervalRow {
	rows := e.rows(word, quizType)
	require.NotEmpty(e.t, rows)
	return rows[0]
}

// seedCorrect records a correct answer (quality 4) through the real write path.
func (e *overrideIntervalEnv) seedCorrect(word string, quizType notebook.QuizType) {
	e.t.Helper()
	if quizType == notebook.QuizTypeReverse {
		cards, err := e.svc.LoadReverseCards(e.userID, []string{overrideIntervalBook}, false, true, nil)
		require.NoError(e.t, err)
		for _, c := range cards {
			if c.Expression == word || c.AltForm == word {
				require.NoError(e.t, e.svc.SaveReverseResult(e.ctx, e.userID, c, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
				return
			}
		}
		e.t.Fatalf("%s is not due for reverse", word)
	}
	cards, err := e.svc.LoadCards(e.userID, []string{overrideIntervalBook}, true, nil)
	require.NoError(e.t, err)
	for _, c := range cards {
		if c.Entry == word || c.OriginalEntry == word {
			require.NoError(e.t, e.svc.SaveResult(e.ctx, e.userID, c, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
			return
		}
	}
	e.t.Fatalf("%s is not due for the standard quiz", word)
}

// answered is what the quiz screen holds after a submit: what it sends back
// on Mark-as-Correct / Undo.
type answered struct {
	noteID         int64
	senseID        string
	learnedAt      string
	nextReviewDate string
	quizType       apiv1.QuizType
	handler        *server.QuizHandler
}

// answer starts a quiz through a fresh handler, answers the word with grade,
// and returns what the result card holds — the same calls the frontend makes.
// A fresh handler per quiz keeps one quiz's session card ids from resolving to
// an earlier quiz's cards (the stores of different quiz types share an id
// range).
func (e *overrideIntervalEnv) answer(word, meaning string, quizType notebook.QuizType, grade inference.ValidateWordFormResponse, acceptSynonym bool) answered {
	e.t.Helper()
	e.grade = grade
	h := server.NewQuizHandler(e.svc)
	h.SetNoteRepository(e.repos.Note)
	if quizType == notebook.QuizTypeReverse {
		start, err := h.StartReverseQuiz(e.ctx, connect.NewRequest(&apiv1.StartReverseQuizRequest{
			NotebookIds: []string{overrideIntervalBook}, IncludeUnstudied: true,
		}))
		require.NoError(e.t, err)
		var noteID int64
		for _, c := range start.Msg.GetFlashcards() {
			if c.GetMeaning() == meaning {
				noteID = c.GetNoteId()
			}
		}
		require.NotZero(e.t, noteID, "%s must be due for reverse", word)
		resp, err := h.BatchSubmitReverseAnswers(e.ctx, connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
			Answers: []*apiv1.SubmitReverseAnswerRequest{{NoteId: noteID, Answer: "some answer", ResponseTimeMs: 5000, AcceptSynonymAsCorrect: acceptSynonym}},
		}))
		require.NoError(e.t, err)
		r := resp.Msg.GetResponses()[0]
		require.NotEmpty(e.t, r.GetLearnedAt())
		return answered{noteID: noteID, senseID: r.GetSenseId(), learnedAt: r.GetLearnedAt(), nextReviewDate: r.GetNextReviewDate(), quizType: apiv1.QuizType_QUIZ_TYPE_REVERSE, handler: h}
	}
	start, err := h.StartQuiz(e.ctx, connect.NewRequest(&apiv1.StartQuizRequest{
		NotebookIds: []string{overrideIntervalBook}, IncludeUnstudied: true,
	}))
	require.NoError(e.t, err)
	var noteID int64
	for _, c := range start.Msg.GetFlashcards() {
		if c.GetEntry() == word || c.GetOriginalEntry() == word {
			noteID = c.GetNoteId()
		}
	}
	require.NotZero(e.t, noteID, "%s must be due for the standard quiz", word)
	resp, err := h.SubmitAnswer(e.ctx, connect.NewRequest(&apiv1.SubmitAnswerRequest{
		NoteId: noteID, Answer: "some answer", ResponseTimeMs: 5000,
	}))
	require.NoError(e.t, err)
	require.NotEmpty(e.t, resp.Msg.GetLearnedAt())
	return answered{noteID: noteID, senseID: resp.Msg.GetSenseId(), learnedAt: resp.Msg.GetLearnedAt(), nextReviewDate: resp.Msg.GetNextReviewDate(), quizType: apiv1.QuizType_QUIZ_TYPE_STANDARD, handler: h}
}

func (e *overrideIntervalEnv) override(a answered, markCorrect bool) *apiv1.OverrideAnswerResponse {
	e.t.Helper()
	resp, err := a.handler.OverrideAnswer(e.ctx, connect.NewRequest(&apiv1.OverrideAnswerRequest{
		NoteId: a.noteID, SenseId: a.senseID, QuizType: a.quizType, LearnedAt: a.learnedAt, MarkCorrect: &markCorrect,
	}))
	require.NoError(e.t, err)
	return resp.Msg
}

// due reports whether the word is due again in quizType, read the way the
// quiz loaders read it (DB history store → NeedsReverseReview/NeedsForwardReview).
func (e *overrideIntervalEnv) due(word string, quizType notebook.QuizType) bool {
	histories, err := e.repos.HistoryStore.LoadForNotebooks(e.ctx, []string{overrideIntervalBook}, e.userID)
	require.NoError(e.t, err)
	// The DB history names an entry by the note's shown surface (its entry).
	var entry string
	require.NoError(e.t, e.db.GetContext(e.ctx, &entry, `SELECT entry FROM notes WHERE "usage" = $1`, word))
	expr := notebook.FindExpressionInHistories(histories[overrideIntervalBook], "", entry)
	require.NotNil(e.t, expr, "%s must have a history", word)
	if quizType == notebook.QuizTypeReverse {
		return expr.NeedsReverseReview()
	}
	return expr.NeedsForwardReview()
}

func plusDays(t *testing.T, learnedAt string, days int) string {
	d, err := time.Parse("2006-01-02", learnedAt)
	require.NoError(t, err)
	return d.AddDate(0, 0, days).Format("2006-01-02")
}

var (
	gradeWrong   = inference.ValidateWordFormResponse{Classification: inference.ClassificationWrong, Reason: "no match", Quality: 1}
	gradeCorrect = inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "match", Quality: 4}
	gradeSynonym = inference.ValidateWordFormResponse{Classification: inference.ClassificationSynonym, Reason: "a synonym", Quality: 2}
)

// overrideIntervalWord is an example word the scenarios run on: its usage, its
// meaning (how a reverse card is found) and the quiz type.
type overrideIntervalWord struct {
	word     string
	meaning  string
	quizType notebook.QuizType
}

var overrideIntervalWords = []overrideIntervalWord{
	{word: "pulmonary", meaning: "relating to the lungs", quizType: notebook.QuizTypeReverse},
	{word: "pulmonary", meaning: "relating to the lungs", quizType: notebook.QuizTypeNotebook},
	// An id-less word (resolved by expression, not sense id).
	{word: "hepatic", meaning: "relating to the liver", quizType: notebook.QuizTypeReverse},
}

// TestOverrideAnswer_RecomputesInterval_LivePostgres_Integration pins that
// Mark-as-Correct / Mark-as-Incorrect reschedule the attempt as if it had been
// graded that way: the interval is replayed from the attempts before it with
// the configured calculator, not kept from the wrong grade (correct) or reset
// to 1 (incorrect). Undo puts the row back exactly.
func TestOverrideAnswer_RecomputesInterval_LivePostgres_Integration(t *testing.T) {
	env := newOverrideIntervalEnv(t)

	for _, w := range overrideIntervalWords {
		qt := string(w.quizType)
		other := string(notebook.QuizTypeNotebook)
		if w.quizType == notebook.QuizTypeNotebook {
			other = string(notebook.QuizTypeReverse)
		}

		t.Run(w.word+"/"+qt+"/mark correct at 7 days advances to 30", func(t *testing.T) {
			env.t = t
			env.reset(w.word)
			env.seedCorrect(w.word, w.quizType) // u q4 → 7
			require.Equal(t, 7, env.latest(w.word, qt).IntervalDays)
			env.age(w.word, 8)

			a := env.answer(w.word, w.meaning, w.quizType, gradeWrong, false)
			wrong := env.latest(w.word, qt)
			require.Equal(t, "misunderstood", wrong.Status)
			require.Equal(t, 1, wrong.Quality)
			require.Equal(t, 1, wrong.IntervalDays, "a wrong answer at 7 days moves one level down (unchanged behavior)")

			resp := env.override(a, true)
			got := env.latest(w.word, qt)
			assert.Equal(t, wrong.ID, got.ID, "the override rewrites the answered row, it doesn't add one")
			assert.Equal(t, "understood", got.Status)
			assert.Equal(t, 4, got.Quality)
			assert.Equal(t, 30, got.IntervalDays, "a correct answer at 7 days advances to 30")
			assert.Equal(t, plusDays(t, a.learnedAt, 30), resp.GetNextReviewDate())
			assert.Equal(t, "misunderstood", resp.GetOriginalStatus())
			assert.Equal(t, int32(1), resp.GetOriginalQuality())
			assert.Equal(t, int32(1), resp.GetOriginalIntervalDays())
			assert.Empty(t, env.rows(w.word, other), "the override touches only its own quiz type's series")

			env.age(w.word, 1)
			assert.False(t, env.due(w.word, w.quizType), "the word must not be due the next day")
		})

		t.Run(w.word+"/"+qt+"/mark correct at 90 days advances to 365", func(t *testing.T) {
			env.t = t
			env.reset(w.word)
			env.seedCorrect(w.word, w.quizType) // → 7
			env.age(w.word, 8)
			env.seedCorrect(w.word, w.quizType) // → 30
			env.age(w.word, 31)
			env.seedCorrect(w.word, w.quizType) // → 90
			require.Equal(t, 90, env.latest(w.word, qt).IntervalDays)
			env.age(w.word, 91)

			a := env.answer(w.word, w.meaning, w.quizType, gradeWrong, false)
			require.Equal(t, 30, env.latest(w.word, qt).IntervalDays, "a wrong answer at 90 days moves one level down")

			resp := env.override(a, true)
			got := env.latest(w.word, qt)
			assert.Equal(t, "understood", got.Status)
			assert.Equal(t, 4, got.Quality)
			assert.Equal(t, 365, got.IntervalDays)
			assert.Equal(t, plusDays(t, a.learnedAt, 365), resp.GetNextReviewDate())

			env.age(w.word, 1)
			assert.False(t, env.due(w.word, w.quizType))
		})

		t.Run(w.word+"/"+qt+"/mark incorrect recomputes the lapse and undo restores it", func(t *testing.T) {
			env.t = t
			env.reset(w.word)
			env.seedCorrect(w.word, w.quizType) // → 7
			env.age(w.word, 8)
			env.seedCorrect(w.word, w.quizType) // → 30
			env.age(w.word, 31)

			a := env.answer(w.word, w.meaning, w.quizType, gradeCorrect, false)
			before := env.latest(w.word, qt)
			require.Equal(t, "understood", before.Status)
			require.Equal(t, 90, before.IntervalDays)
			require.Equal(t, plusDays(t, a.learnedAt, 90), a.nextReviewDate)

			resp := env.override(a, false)
			got := env.latest(w.word, qt)
			assert.Equal(t, "misunderstood", got.Status)
			assert.Equal(t, 1, got.Quality)
			assert.Equal(t, 7, got.IntervalDays, "a wrong answer after 30 days moves one level down to 7, not a flat 1")
			assert.Equal(t, plusDays(t, a.learnedAt, 7), resp.GetNextReviewDate())

			undo, err := a.handler.UndoOverrideAnswer(env.ctx, connect.NewRequest(&apiv1.UndoOverrideAnswerRequest{
				NoteId: a.noteID, SenseId: a.senseID, QuizType: a.quizType, LearnedAt: a.learnedAt,
				OriginalQuality: resp.GetOriginalQuality(), OriginalStatus: resp.GetOriginalStatus(), OriginalIntervalDays: resp.GetOriginalIntervalDays(),
			}))
			require.NoError(t, err)
			restored := env.latest(w.word, qt)
			assert.Equal(t, before, restored, "undo restores the row exactly")
			assert.True(t, undo.Msg.GetCorrect())
			assert.Equal(t, a.nextReviewDate, undo.Msg.GetNextReviewDate())
		})
	}
}

// TestAcceptedSynonym_HoldsInterval_LivePostgres_Integration pins that a
// reverse answer accepted as a synonym on retry (saved understood, quality 2)
// keeps the word's interval instead of scheduling it as a lapse.
func TestAcceptedSynonym_HoldsInterval_LivePostgres_Integration(t *testing.T) {
	env := newOverrideIntervalEnv(t)
	const word, meaning = "pulmonary", "relating to the lungs"
	qt := string(notebook.QuizTypeReverse)

	t.Run("on time after 7 days holds 7", func(t *testing.T) {
		env.t = t
		env.reset(word)
		env.seedCorrect(word, notebook.QuizTypeReverse) // → 7
		env.age(word, 8)

		a := env.answer(word, meaning, notebook.QuizTypeReverse, gradeSynonym, true)
		got := env.latest(word, qt)
		assert.Equal(t, "understood", got.Status)
		assert.Equal(t, 2, got.Quality)
		assert.Equal(t, 7, got.IntervalDays, "an accepted synonym holds the level instead of dropping to 1")
		assert.Equal(t, plusDays(t, a.learnedAt, 7), a.nextReviewDate)

		env.age(word, 1)
		assert.False(t, env.due(word, notebook.QuizTypeReverse))
	})

	t.Run("no history gets the first level", func(t *testing.T) {
		env.t = t
		env.reset(word)
		env.answer(word, meaning, notebook.QuizTypeReverse, gradeSynonym, true)
		got := env.latest(word, qt)
		assert.Equal(t, "understood", got.Status)
		assert.Equal(t, 1, got.IntervalDays)
	})
}

// TestRepairLatestIntervals_LivePostgres_Integration seeds rows the way the
// old override left them (a correct answer stored with the wrong answer's
// interval) and runs the real `migrate repair-intervals` command: without
// --apply it only reports, with --apply it rewrites only the newest row of each
// broken series, and a second run changes nothing.
func TestRepairLatestIntervals_LivePostgres_Integration(t *testing.T) {
	env := newOverrideIntervalEnv(t)
	dsn, err := url.Parse(os.Getenv("LANGNER_INTEGRATION_DB_URL"))
	require.NoError(t, err)
	password, _ := dsn.User.Password()
	t.Setenv("DB_HOST", dsn.Hostname())
	t.Setenv("DB_PORT", dsn.Port())
	t.Setenv("DB_NAME", strings.TrimPrefix(dsn.Path, "/"))
	t.Setenv("DB_USER", dsn.User.Username())
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("QUIZ_ALGORITHM", "fixed")
	t.Setenv("QUIZ_FIXED_INTERVALS", "1,7,30,90,365,1095,1825")
	setConfigFile(t, "config.example.yml")
	repair := func(args ...string) string {
		var out strings.Builder
		cmd := newMigrateRepairIntervalsCommand()
		cmd.SetOut(&out)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, cmd.RunE(cmd, nil))
		return out.String()
	}

	// Bring the imported example history in line first, so the counts below
	// are only the rows this test breaks.
	t.Log(repair("--apply"))

	const word = "pulmonary"
	env.reset(word)
	for _, qt := range []notebook.QuizType{notebook.QuizTypeReverse, notebook.QuizTypeNotebook} {
		env.seedCorrect(word, qt) // → 7
	}
	env.age(word, 8)
	for _, qt := range []notebook.QuizType{notebook.QuizTypeReverse, notebook.QuizTypeNotebook} {
		env.seedCorrect(word, qt) // → 30
	}
	for _, qt := range []string{"reverse", "notebook"} {
		rows := env.rows(word, qt)
		require.Len(t, rows, 2)
		require.Equal(t, 30, rows[0].IntervalDays)
		_, err := env.db.ExecContext(env.ctx, `UPDATE learning_logs SET interval_days = 1 WHERE id = $1`, rows[0].ID)
		require.NoError(t, err)
	}

	assert.Contains(t, repair(), "Would repair the interval of 2 learning log(s).")
	for _, qt := range []string{"reverse", "notebook"} {
		assert.Equal(t, 1, env.rows(word, qt)[0].IntervalDays, "%s: a report-only run writes nothing", qt)
	}

	assert.Contains(t, repair("--apply"), "Repaired the interval of 2 learning log(s).")
	for _, qt := range []string{"reverse", "notebook"} {
		rows := env.rows(word, qt)
		assert.Equal(t, 30, rows[0].IntervalDays, "%s: the newest row is replayed to 30", qt)
		assert.Equal(t, 7, rows[1].IntervalDays, "%s: older rows are left alone", qt)
	}
	env.age(word, 1)
	assert.False(t, env.due(word, notebook.QuizTypeReverse))
	assert.False(t, env.due(word, notebook.QuizTypeNotebook))

	assert.Contains(t, repair("--apply"), "Repaired the interval of 0 learning log(s).", "a second run changes nothing")

	n, err := learning.RepairLatestIntervals(env.ctx, env.db, notebook.NewIntervalCalculator("fixed", overrideIntervalLadder), false)
	require.NoError(t, err)
	assert.Zero(t, n)
}
