package clicmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/dbseed"
)

// e2eHistoryFixture is one seeded learning attempt for the e2e suite. Exactly
// one target kind is implied: origin=true → etymology-origin log; grammar=true →
// grammar log (Expression is the sense_id); otherwise a vocabulary note log.
type e2eHistoryFixture struct {
	Notebook string
	Word     string // note expression, origin string, or grammar sense_id
	QuizType string
	Status   string
	Day      string // YYYY-MM-DD
	Quality  int
	Origin   bool
	Grammar  bool
	// Title is the on-disk notebook's display title, carried onto the note's
	// notebook_notes membership so the reconstructed LearningHistory matches
	// the content the quiz loaders read BY TITLE (dbseed.LearningLogSpec.Group).
	// Required for vocab (note) fixtures; ignored for origin/grammar.
	Title string
	// SenseID is the vocab card's content-derived stable id (a flashcard `id:`),
	// "" for an id-less card keyed by (usage, entry). It MUST match the on-disk
	// content so the seeded note is the SAME row the runtime ensure-on-serve
	// resolves — otherwise a runtime answer forks a membership-less duplicate
	// note and the word vanishes from Relearn / reconstruction reads.
	SenseID string
}

// e2eHistoryFixtures replaces the old frontend/e2e/fixtures/learning_notes YAML.
// It is the exact dated history the e2e scenarios assert on (analytics Day
// Detail, reverse-progress, quiz start counts), expressed as flat rows seeded
// through the real write path via internal/dbseed — no misleading
// learned_logs/reverse_logs/etymology_origin_logs YAML shape.
var e2eHistoryFixtures = []e2eHistoryFixture{
	// idioms (flashcard): a wrong notebook+reverse attempt on 01-02, a correct
	// freeform recall on 01-03 — the analytics Day Detail "break the ice" case.
	{Notebook: "idioms", Title: "Common Idioms", SenseID: "break-the-ice", Word: "break the ice", QuizType: "notebook", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Title: "Common Idioms", SenseID: "break-the-ice", Word: "break the ice", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Title: "Common Idioms", SenseID: "break-the-ice", Word: "break the ice", QuizType: "freeform", Status: "understood", Day: "2025-01-03", Quality: 4},
	{Notebook: "idioms", Title: "Common Idioms", Word: "lose one's temper", QuizType: "notebook", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Title: "Common Idioms", Word: "lose one's temper", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Title: "Common Idioms", Word: "lose one's temper", QuizType: "freeform", Status: "understood", Day: "2025-01-03", Quality: 4},

	// reverse-progress (flashcard): learned on 01-01, missed in reverse on 01-02.
	{Notebook: "reverse-progress", Title: "Reverse Progress", Word: "spick and span", QuizType: "freeform", Status: "understood", Day: "2025-01-01", Quality: 4},
	{Notebook: "reverse-progress", Title: "Reverse Progress", Word: "spick and span", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "reverse-progress", Title: "Reverse Progress", Word: "under the weather", QuizType: "freeform", Status: "understood", Day: "2025-01-01", Quality: 4},
	{Notebook: "reverse-progress", Title: "Reverse Progress", Word: "under the weather", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},

	// practice (grammar): a missed grammar correction — the analytics
	// "labeled grammar, not notebook" case (Word is the correction sense_id).
	{Notebook: "practice", Word: "party-suggested", QuizType: "grammar", Status: "misunderstood", Day: "2025-01-02", Quality: 1, Grammar: true},

	// etymology origins missed on 01-02 — the analytics "I see 'scribo'" case.
	{Notebook: "word-roots", Word: "graph", QuizType: "etymology_origin", Status: "misunderstood", Day: "2025-01-02", Quality: 1, Origin: true},
	{Notebook: "word-roots", Word: "tele", QuizType: "etymology_origin", Status: "misunderstood", Day: "2025-01-02", Quality: 1, Origin: true},
	{Notebook: "word-stems", Word: "scribo", QuizType: "etymology_origin", Status: "misunderstood", Day: "2025-01-02", Quality: 1, Origin: true},
	{Notebook: "word-stems", Word: "dico", QuizType: "etymology_origin", Status: "misunderstood", Day: "2025-01-02", Quality: 1, Origin: true},
}

// NewMigrateSeedE2ECommand seeds the deterministic learning-history the e2e
// suite asserts on, through the real write path (internal/dbseed) — replacing
// the reset-db/import seed for e2e and the learning_notes YAML fixtures. Notebook
// CONTENT is served from the on-disk fixtures (config.e2e.yml); this command
// only seeds STATE (history), attributed to the initial-admin account. Runs once
// on a fresh DB at server start; the per-scenario reset uses reset-e2e.
func NewMigrateSeedE2ECommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed-e2e",
		Short: "Seed deterministic e2e learning history (state only) via the fixtures library",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			ownerID, err := resolveSeedOwnerID(ctx, cfg, db)
			if err != nil {
				return fmt.Errorf("resolve seed owner: %w", err)
			}
			if err := seedE2EHistory(ctx, db, ownerID); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Seeded %d e2e learning-history rows.\n", len(e2eHistoryFixtures))
			return nil
		},
	}
}

// e2eStateTables are the learning-content STATE tables seed-e2e populates and a
// quiz run mutates. reset-e2e truncates exactly these back to the seeded
// baseline; CASCADE clears their dependents (note_images / note_references /
// note_origin_parts / etymology_origin_forms). users, notebooks (ownership), the
// CLI auth tables, and schema_migrations are deliberately preserved so the
// pre-minted access token keeps resolving (content is filesystem-served, so the
// definitions/flashcard/concept content tables are never populated in e2e).
var e2eStateTables = []string{
	"learning_logs",
	"notebook_notes",
	"note_skip_flags",
	"origin_skip_flags",
	"grammar_corrections",
	"etymology_origins",
	"notes",
}

// NewMigrateResetE2ECommand restores the seeded e2e baseline between scenarios:
// it TRUNCATEs the learning-content STATE tables (undoing the prior scenario's
// quiz writes) and re-seeds the fixtures via the same fixtures-library path as
// seed-e2e — with NO import pipeline (reset-db) and without touching auth, so
// the harness's access token stays valid and no re-provision is needed.
func NewMigrateResetE2ECommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-e2e",
		Short: "Reset e2e learning state to the seeded baseline (truncate + fixtures reseed)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			ownerID, err := resolveSeedOwnerID(ctx, cfg, db)
			if err != nil {
				return fmt.Errorf("resolve seed owner: %w", err)
			}
			if _, err := db.ExecContext(ctx,
				"TRUNCATE "+strings.Join(e2eStateTables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
				return fmt.Errorf("truncate e2e state: %w", err)
			}
			if err := seedE2EHistory(ctx, db, ownerID); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reset e2e state and re-seeded %d learning-history rows.\n", len(e2eHistoryFixtures))
			return nil
		},
	}
}

// seedE2EHistory inserts every e2eHistoryFixtures row through the fixtures
// library (internal/dbseed) — the real write path — attributed to ownerID.
func seedE2EHistory(ctx context.Context, db *sqlx.DB, ownerID int64) error {
	for _, f := range e2eHistoryFixtures {
		day, perr := time.Parse("2006-01-02", f.Day)
		if perr != nil {
			return fmt.Errorf("bad fixture day %q: %w", f.Day, perr)
		}
		spec := dbseed.LearningLogSpec{
			UserID:     ownerID,
			NotebookID: f.Notebook,
			Group:      f.Title,
			QuizType:   f.QuizType,
			Status:     f.Status,
			Quality:    f.Quality,
			LearnedAt:  day,
		}
		switch {
		case f.Origin:
			originID, oerr := dbseed.SeedEtymologyOrigin(ctx, db, f.Notebook, f.Word, "", "", "")
			if oerr != nil {
				return oerr
			}
			spec.OriginID = originID
		case f.Grammar:
			spec.SenseID = f.Word
		default:
			spec.Expression = f.Word
			spec.SenseID = f.SenseID // the note's content id ("" = id-less)
		}
		if err := dbseed.SeedLearningLog(ctx, db, spec); err != nil {
			return err
		}
	}
	return nil
}
