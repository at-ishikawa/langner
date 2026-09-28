package clicmd

import (
	"fmt"
	"time"

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
}

// e2eHistoryFixtures replaces the old frontend/e2e/fixtures/learning_notes YAML.
// It is the exact dated history the e2e scenarios assert on (analytics Day
// Detail, reverse-progress, quiz start counts), expressed as flat rows seeded
// through the real write path via internal/dbseed — no misleading
// learned_logs/reverse_logs/etymology_origin_logs YAML shape.
var e2eHistoryFixtures = []e2eHistoryFixture{
	// idioms (flashcard): a wrong notebook+reverse attempt on 01-02, a correct
	// freeform recall on 01-03 — the analytics Day Detail "break the ice" case.
	{Notebook: "idioms", Word: "break the ice", QuizType: "notebook", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Word: "break the ice", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Word: "break the ice", QuizType: "freeform", Status: "understood", Day: "2025-01-03", Quality: 4},
	{Notebook: "idioms", Word: "lose one's temper", QuizType: "notebook", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Word: "lose one's temper", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "idioms", Word: "lose one's temper", QuizType: "freeform", Status: "understood", Day: "2025-01-03", Quality: 4},

	// reverse-progress (flashcard): learned on 01-01, missed in reverse on 01-02.
	{Notebook: "reverse-progress", Word: "spick and span", QuizType: "freeform", Status: "understood", Day: "2025-01-01", Quality: 4},
	{Notebook: "reverse-progress", Word: "spick and span", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},
	{Notebook: "reverse-progress", Word: "under the weather", QuizType: "freeform", Status: "understood", Day: "2025-01-01", Quality: 4},
	{Notebook: "reverse-progress", Word: "under the weather", QuizType: "reverse", Status: "misunderstood", Day: "2025-01-02", Quality: 1},

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
// only seeds STATE (history), attributed to the initial-admin account.
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

			for _, f := range e2eHistoryFixtures {
				day, perr := time.Parse("2006-01-02", f.Day)
				if perr != nil {
					return fmt.Errorf("bad fixture day %q: %w", f.Day, perr)
				}
				spec := dbseed.LearningLogSpec{
					UserID:     ownerID,
					NotebookID: f.Notebook,
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
				}
				if err := dbseed.SeedLearningLog(ctx, db, spec); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Seeded %d e2e learning-history rows.\n", len(e2eHistoryFixtures))
			return nil
		},
	}
}
