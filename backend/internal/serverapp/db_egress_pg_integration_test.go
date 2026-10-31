package serverapp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/clicmd"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/serverapp"
	"github.com/at-ishikawa/langner/schemas"
)

// fullHistoryStore hides DBHistoryStore's scoped loaders (summary / skip-state)
// so a Service built on it reads every learning log, as before those loaders.
type fullHistoryStore struct {
	learning.HistoryStore
}

// egressFixture is the example catalog imported into a fresh Postgres, the way
// the server reads it: config.example.yml's notebooks in notebook_files, notes
// created on serve, plus a seeded learning history.
type egressFixture struct {
	cfg    *config.Config
	db     *sqlx.DB
	userID int64
	dsn    string
}

func setupEgressFixture(t *testing.T) egressFixture {
	t.Helper()
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	// Run from the repo root so config.example.yml's relative notebook dirs
	// resolve, exactly as the server does.
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

	ctx := context.Background()
	_, err = clicmd.ImportShippedContent(ctx, cfg, db, io.Discard)
	require.NoError(t, err)

	var userID int64
	require.NoError(t, db.GetContext(ctx, &userID,
		`INSERT INTO users (google_sub, username) VALUES ('egress-test', 'egress-test') RETURNING id`))
	return egressFixture{cfg: cfg, db: db, userID: userID, dsn: dsn}
}

// newService builds the quiz service the way serverapp.BuildHandler does in DB
// mode (DB history store, DB content source, ensure-on-serve, visibility ACL).
func newService(t *testing.T, f egressFixture, fullHistory bool) *quiz.Service {
	t.Helper()
	repos := bootstrap.BuildStateRepositories(f.cfg.Notebooks, f.cfg.Quiz, f.db)
	contentSource := notebook.NewDBContentSource(notebook.NewNotebookFileRepository(f.db))
	dirs, err := contentSource.Dirs(context.Background())
	require.NoError(t, err)

	svc := quiz.NewService(f.cfg.Notebooks, inference.StaticResolver(mock.NewClient()),
		map[string]rapidapi.Response{}, repos.Learning, f.cfg.Quiz)
	var store learning.HistoryStore = repos.HistoryStore
	if fullHistory {
		store = fullHistoryStore{repos.HistoryStore}
	}
	svc.SetHistoryStore(store)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetContentSource(contentSource)
	svc.SetUserDefinitionsRepo(notebook.NewUserDefinitionsRepository(f.db))

	ensureReader, err := notebook.NewReader(dirs.Stories, dirs.Flashcards, dirs.Books, dirs.Definitions, dirs.Etymology, nil)
	require.NoError(t, err)
	require.NoError(t, ensureReader.LoadJournals(dirs.Journals))
	require.NoError(t, ensureReader.LoadGrammars(dirs.Grammars))
	svc.SetNoteEnsurer(datasync.NewImporter(repos.Note, nil, notebook.NewYAMLNoteRepository(ensureReader), nil, nil, nil, io.Discard))
	svc.SetNotebookACL(repos.ACL)
	return svc
}

// seedHistory creates every example word's note (loading its cards, as a quiz
// start does) and writes a varied learning history through the DB repositories:
// for each note and quiz type, 0-6 logs with mixed results, ages and intervals
// (some legacy 0-day intervals), attributed to one of the note's notebooks or
// to none, plus a few freeform exclusions.
func seedHistory(t *testing.T, f egressFixture, svc *quiz.Service) {
	t.Helper()
	ctx := context.Background()
	reader, err := svc.NewReader()
	require.NoError(t, err)
	var ids []string
	for id := range reader.GetStoryIndexes() {
		ids = append(ids, id)
	}
	for id := range reader.GetFlashcardIndexes() {
		ids = append(ids, id)
	}
	ids = append(ids, reader.GetDefinitionsBookIDs()...)
	sort.Strings(ids)
	_, err = svc.LoadCards(f.userID, ids, true, nil)
	require.NoError(t, err)

	var links []struct {
		NoteID     int64  `db:"note_id"`
		NotebookID string `db:"notebook_id"`
	}
	require.NoError(t, f.db.SelectContext(ctx, &links,
		`SELECT note_id, notebook_id FROM notebook_notes ORDER BY note_id, notebook_id`))
	require.NotEmpty(t, links, "loading the example cards must create their notes")
	notebooksByNote := map[int64][]string{}
	var noteIDs []int64
	for _, l := range links {
		if _, ok := notebooksByNote[l.NoteID]; !ok {
			noteIDs = append(noteIDs, l.NoteID)
		}
		notebooksByNote[l.NoteID] = append(notebooksByNote[l.NoteID], l.NotebookID)
	}

	rng := rand.New(rand.NewSource(42))
	statuses := []string{"misunderstood", "understood", "usable", "intuitive", ""}
	intervals := []int{0, 1, 3, 7, 30, 90, 365}
	var logs []*learning.LearningLog
	for _, noteID := range noteIDs {
		for _, quizType := range []string{"notebook", "reverse", "freeform"} {
			for range rng.Intn(7) {
				source := ""
				if nbs := notebooksByNote[noteID]; rng.Intn(4) > 0 {
					source = nbs[rng.Intn(len(nbs))]
				}
				logs = append(logs, &learning.LearningLog{
					UserID:           f.userID,
					NoteID:           noteID,
					Status:           statuses[rng.Intn(len(statuses))],
					LearnedAt:        time.Now().Add(-time.Duration(rng.Intn(400*24)) * time.Hour),
					Quality:          rng.Intn(6),
					QuizType:         quizType,
					IntervalDays:     intervals[rng.Intn(len(intervals))],
					SourceNotebookID: source,
				})
			}
		}
	}
	require.NoError(t, learning.NewDBLearningRepository(f.db).BatchCreate(ctx, logs))

	skips := notebook.NewDBSkipFlagRepository(f.db)
	for i, noteID := range noteIDs {
		if i%5 == 0 {
			require.NoError(t, skips.SkipNote(ctx, f.userID, noteID, "freeform", time.Now()))
		}
	}
}

// TestDBEgress_ScopedLoadsMatchFullHistory_LivePostgres_Integration pins that
// the reduced reads give the SAME answers as reading the full history: the
// quiz options summary (newest + newest-correct logs only) and the all-words
// list behind the freeform quiz and Relearn (skip state only, no logs).
func TestDBEgress_ScopedLoadsMatchFullHistory_LivePostgres_Integration(t *testing.T) {
	f := setupEgressFixture(t)
	scoped := newService(t, f, false)
	full := newService(t, f, true)
	seedHistory(t, f, scoped)

	for _, includeUnstudied := range []bool{false, true} {
		want, err := full.LoadNotebookSummaries(f.userID, includeUnstudied)
		require.NoError(t, err)
		got, err := scoped.LoadNotebookSummaries(f.userID, includeUnstudied)
		require.NoError(t, err)
		require.NotEmpty(t, want)
		assert.Equal(t, sortedSummaries(want), sortedSummaries(got), "includeUnstudied=%v", includeUnstudied)
	}

	wantWords, err := full.LoadAllWords(f.userID)
	require.NoError(t, err)
	gotWords, err := scoped.LoadAllWords(f.userID)
	require.NoError(t, err)
	require.NotEmpty(t, wantWords)
	assert.Equal(t, sortedWords(wantWords), sortedWords(gotWords))

	// The summary read really is smaller: the seeded history must exercise it.
	var allLogs, summaryLogs int
	require.NoError(t, f.db.Get(&allLogs, `SELECT count(*) FROM learning_logs`))
	var noteIDs []int64
	require.NoError(t, f.db.Select(&noteIDs, `SELECT DISTINCT note_id FROM notebook_notes`))
	got, err := learning.NewDBLearningRepository(f.db).FindSummaryByTargets(context.Background(), noteIDs, nil, nil, f.userID)
	require.NoError(t, err)
	summaryLogs = len(got)
	assert.Less(t, summaryLogs, allLogs, "the summary read must drop logs scheduling never reads")
}

func sortedSummaries(s []quiz.NotebookSummary) []quiz.NotebookSummary {
	out := append([]quiz.NotebookSummary(nil), s...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].NotebookID != out[j].NotebookID {
			return out[i].NotebookID < out[j].NotebookID
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func sortedWords(w []quiz.FreeformCard) []quiz.FreeformCard {
	out := append([]quiz.FreeformCard(nil), w...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.NotebookName != b.NotebookName {
			return a.NotebookName < b.NotebookName
		}
		if a.StoryTitle != b.StoryTitle {
			return a.StoryTitle < b.StoryTitle
		}
		if a.SceneTitle != b.SceneTitle {
			return a.SceneTitle < b.SceneTitle
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Expression < b.Expression
	})
	return out
}

// findRepoRoot returns the directory containing config.example.yml — the root
// the running server is launched from.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != filepath.Dir(path); path = filepath.Dir(path) {
		if _, err := os.Stat(filepath.Join(path, "config.example.yml")); err == nil {
			return path
		}
	}
	t.Fatalf("config.example.yml not found above %s", wd)
	return ""
}

// TestDBEgress_ServerReadsLittleFromPostgres_LivePostgres_Integration drives
// the real server (BuildHandler from config.example.yml) and checks, from its
// "db usage" log lines, that:
//   - startup reads notebook content in ONE query, compressed, and backfills a
//     compressed copy for rows written before migration 037;
//   - the per-request content fingerprint is a single short row;
//   - the quiz options summary reads only the scheduling logs;
//   - a batch submit loads the quizzed notebook once, not once per answer;
//   - starting Relearn reads no full per-notebook histories.
func TestDBEgress_ServerReadsLittleFromPostgres_LivePostgres_Integration(t *testing.T) {
	f := setupEgressFixture(t)
	ctx := context.Background()

	u, err := url.Parse(f.dsn)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	password, _ := u.User.Password()
	f.cfg.Database = config.DatabaseConfig{Host: u.Hostname(), Port: port, Database: strings.TrimPrefix(u.Path, "/"), Username: u.User.Username(), Password: password}
	f.cfg.Auth.TokenSigningKey = "db-egress-test-signing-key"
	f.cfg.Inference.Mode = "mock"

	token, err := clicmd.IssueTestToken(ctx, f.cfg, f.db, "egress@example.com")
	require.NoError(t, err)
	require.NoError(t, f.db.Get(&f.userID, `SELECT id FROM users WHERE google_sub = 'e2e-test|egress@example.com'`))
	seedHistory(t, f, newService(t, f, false))
	// A few recent misses so Relearn has a pool.
	var recent []int64
	require.NoError(t, f.db.Select(&recent, `SELECT DISTINCT note_id FROM notebook_notes ORDER BY note_id LIMIT 5`))
	for _, noteID := range recent {
		require.NoError(t, learning.NewDBLearningRepository(f.db).Create(ctx, &learning.LearningLog{
			UserID: f.userID, NoteID: noteID, Status: "misunderstood", LearnedAt: time.Now().Add(-time.Hour), QuizType: "notebook", IntervalDays: 1,
		}))
	}

	// Rows written before migration 037 have no compressed copy.
	var totalContentBytes int64
	require.NoError(t, f.db.Get(&totalContentBytes, `SELECT sum(byte_size) FROM notebook_files`))
	var legacyNotebook string
	require.NoError(t, f.db.Get(&legacyNotebook,
		`SELECT notebook_id FROM notebook_files GROUP BY notebook_id ORDER BY sum(byte_size), notebook_id LIMIT 1`))
	_, err = f.db.Exec(`UPDATE notebook_files SET content_gz = NULL WHERE notebook_id = $1`, legacyNotebook)
	require.NoError(t, err)

	logs := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	handler, _, cleanup, err := serverapp.BuildHandler(f.cfg)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	// Startup: one compressed content read; the legacy rows got their copy.
	contentReads := usageLines(t, logs, "db usage (unattributed query)", func(l map[string]any) bool {
		return strings.Contains(l["sql"].(string), "content_gz") && strings.Contains(l["sql"].(string), "FROM notebook_files ORDER BY")
	})
	require.Len(t, contentReads, 1, "startup must read every notebook's content in one query")
	assert.Less(t, contentReads[0]["bytes_in"].(float64), float64(totalContentBytes),
		"the content read must transfer the compressed copies, not the raw files")
	var compressedBytes int64
	require.NoError(t, f.db.Get(&compressedBytes, `SELECT sum(length(content_gz)) FROM notebook_files`))
	assert.Less(t, compressedBytes, totalContentBytes*6/10, "the stored copies must be compressed")
	var missingCopies int
	require.NoError(t, f.db.Get(&missingCopies, `SELECT count(*) FROM notebook_files WHERE content_gz IS NULL`))
	assert.Zero(t, missingCopies, "reading must backfill the compressed copy of older rows")

	call := func(h http.Handler, procedure, body string) []byte {
		req := httptest.NewRequest(http.MethodPost, "/api.v1.QuizService/"+procedure, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", procedure, rec.Body.String())
		return rec.Body.Bytes()
	}
	requestUsage := func(procedure string) map[string]any {
		lines := usageLines(t, logs, "db usage", func(l map[string]any) bool {
			return l["endpoint"] == "/api.v1.QuizService/"+procedure
		})
		require.NotEmpty(t, lines, procedure)
		return lines[len(lines)-1]
	}

	// Quiz options: a one-row fingerprint and the scheduling-only log read.
	var options struct {
		Notebooks []struct {
			NotebookID string `json:"notebookId"`
			Kind       string `json:"kind"`
		} `json:"notebooks"`
	}
	require.NoError(t, json.Unmarshal(call(handler, "GetQuizOptions", `{}`), &options))
	require.NotEmpty(t, options.Notebooks)
	usage := requestUsage("GetQuizOptions")
	assert.False(t, hasShape(usage, "FROM notebook_files", func(s shape) bool { return s.rows > s.calls }),
		"the content fingerprint must be one row per call: %v", usage["top"])
	assert.True(t, hasShape(usage, "source_notebook_id FROM ( SELECT", nil),
		"the options summary must read the scheduling logs only: %v", usage["top"])

	// Batch reverse submit on a fresh handler (another serverless instance: no
	// cards cached in memory), so every answer resolves from the database.
	var reverse struct {
		Flashcards []struct {
			NoteID       string `json:"noteId"`
			NotebookName string `json:"notebookName"`
		} `json:"flashcards"`
	}
	// Cards without a sense id can't be re-resolved on another instance (a
	// known limitation, unrelated here), so quiz a notebook whose words have ids.
	var withIDs []string
	require.NoError(t, f.db.Select(&withIDs, `SELECT nn.notebook_id FROM notebook_notes nn JOIN notes n ON n.id = nn.note_id
		GROUP BY nn.notebook_id HAVING bool_and(COALESCE(n.sense_id, '') <> '') ORDER BY count(*) DESC`))
	var quizzed string
	for _, id := range withIDs {
		body := call(handler, "StartReverseQuiz", `{"notebookIds":["`+id+`"],"includeUnstudied":true}`)
		require.NoError(t, json.Unmarshal(body, &reverse))
		if len(reverse.Flashcards) >= 3 {
			quizzed = id
			break
		}
	}
	require.NotEmpty(t, quizzed, "an example notebook must have at least 3 reverse cards")
	answers := make([]string, 0, len(reverse.Flashcards))
	for _, c := range reverse.Flashcards {
		answers = append(answers, `{"noteId":"`+c.NoteID+`","notebookName":"`+c.NotebookName+`","answer":"x"}`)
	}
	coldHandler, _, coldCleanup, err := serverapp.BuildHandler(f.cfg)
	require.NoError(t, err)
	t.Cleanup(coldCleanup)
	var submitted struct {
		Responses []struct {
			SenseID string `json:"senseId"`
		} `json:"responses"`
	}
	require.NoError(t, json.Unmarshal(call(coldHandler, "BatchSubmitReverseAnswers", `{"answers":[`+strings.Join(answers, ",")+`]}`), &submitted))
	require.Len(t, submitted.Responses, len(answers))
	for i, r := range submitted.Responses {
		assert.NotEmpty(t, r.SenseID, "answer %d must resolve to its card", i)
	}
	usage = requestUsage("BatchSubmitReverseAnswers")
	assert.LessOrEqual(t, shapeCalls(usage, "COALESCE(note_id, 0) AS note_id, COALESCE(origin_id, 0)"), 2,
		"a %d-answer batch must load the notebook's history once (plus the save preload), not once per answer: %v", len(answers), usage["top"])

	// Relearn: its one recent-window log read, and no per-notebook history
	// reads (the word list only needs skip state, which has no logs).
	call(handler, "StartRelearnQuiz", `{"windowHours":24}`)
	usage = requestUsage("StartRelearnQuiz")
	assert.LessOrEqual(t, shapeCalls(usage, "COALESCE(note_id, 0) AS note_id, COALESCE(origin_id, 0)"), 1,
		"starting Relearn must read only its recent-window logs: %v", usage["top"])
}

type shape struct {
	sql   string
	calls int
	rows  int
}

// shapes parses a "db usage" line's top query shapes.
func shapes(usage map[string]any) []shape {
	var out []shape
	for _, raw := range usage["top"].([]any) {
		s := raw.(string)
		var sh shape
		_, _ = fmt.Sscanf(s[strings.Index(s, "calls="):], "calls=%d rows=%d", &sh.calls, &sh.rows)
		sh.sql = s[strings.Index(s, "sql=")+4:]
		out = append(out, sh)
	}
	return out
}

func hasShape(usage map[string]any, sqlPart string, match func(shape) bool) bool {
	for _, s := range shapes(usage) {
		if strings.Contains(s.sql, sqlPart) && (match == nil || match(s)) {
			return true
		}
	}
	return false
}

func shapeCalls(usage map[string]any, sqlPart string) int {
	n := 0
	for _, s := range shapes(usage) {
		if strings.Contains(s.sql, sqlPart) {
			n += s.calls
		}
	}
	return n
}

// usageLines returns the JSON log lines with msg that satisfy match.
func usageLines(t *testing.T, logs *bytes.Buffer, msg string, match func(map[string]any) bool) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range strings.Split(logs.String(), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &line))
		if line["msg"] == msg && match(line) {
			out = append(out, line)
		}
	}
	return out
}
