package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// newQuizEtymologyStatusCommand prints, for each etymology notebook session,
// how many origins exist, how many have been answered in freeform, how many
// are eligible for standard/reverse quizzes, and how many are due now.
//
// Use this when the standard or reverse quiz looks empty: it shows whether
// origins are blocked by the freeform-first gate (need a freeform answer)
// or merely scheduled for a future review.
func newQuizEtymologyStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "etymology-status",
		Short: "Show eligibility and due counts per etymology session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			histories, err := notebook.NewLearningHistories(cfg.Notebooks.LearningNotesDirectory)
			if err != nil {
				return fmt.Errorf("load learning histories: %w", err)
			}
			for _, dir := range cfg.Notebooks.EtymologyDirectories {
				if err := walkEtymologyStatus(dir, histories); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

type etymologyStatusIndex struct {
	ID            string   `yaml:"id"`
	Kind          string   `yaml:"kind"`
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`
}

type etymologyStatusSession struct {
	Metadata struct {
		Title string `yaml:"title"`
	} `yaml:"metadata"`
	Origins []notebook.EtymologyOrigin `yaml:"origins"`
}

func walkEtymologyStatus(dir string, histories map[string][]notebook.LearningHistory) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(path) != "index.yml" {
			return nil
		}
		var idx etymologyStatusIndex
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if yaml.Unmarshal(data, &idx) != nil || idx.Kind != "Etymology" {
			return nil
		}
		printEtymologyStatus(filepath.Dir(path), idx, histories[idx.ID])
		return nil
	})
}

func printEtymologyStatus(dir string, idx etymologyStatusIndex, history []notebook.LearningHistory) {
	fmt.Printf("\n=== %s (%s) ===\n", idx.Name, idx.ID)
	fmt.Printf("%-15s %5s %5s %5s %5s   %s\n",
		"Session", "total", "frfm", "elig", "due", "blocked-by-future")

	for _, p := range idx.NotebookPaths {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			continue
		}
		var sf etymologyStatusSession
		if yaml.Unmarshal(data, &sf) != nil {
			continue
		}
		title := sf.Metadata.Title
		if title == "" {
			title = filepath.Base(p)
		}

		seen := make(map[string]bool)
		var total, freeformed, eligible, due int
		var blocked []string
		for _, o := range sf.Origins {
			key := strings.ToLower(strings.TrimSpace(o.Origin))
			if seen[key] {
				continue
			}
			seen[key] = true
			total++
			expr := findEtymologyExpression(history, idx.Name, title, o.Origin)
			if expr == nil {
				continue
			}
			if !expr.HasEtymologyFreeformAnswer() {
				continue
			}
			freeformed++
			if !expr.HasCorrectEtymologyAnswer() {
				continue
			}
			eligible++
			if expr.NeedsEtymologyReview(notebook.QuizTypeEtymologyStandard) {
				due++
				continue
			}
			if d := nextEtymologyReviewDate(expr.EtymologyBreakdownLogs); d != "" {
				blocked = append(blocked, fmt.Sprintf("%s→%s", o.Origin, d))
			}
		}
		sort.Strings(blocked)
		fmt.Printf("%-15s %5d %5d %5d %5d   %s\n",
			title, total, freeformed, eligible, due, strings.Join(blocked, ", "))
	}
}

func findEtymologyExpression(hist []notebook.LearningHistory, notebookTitle, sceneTitle, origin string) *notebook.LearningHistoryExpression {
	for _, h := range hist {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for i := range scene.Expressions {
				if strings.EqualFold(scene.Expressions[i].Expression, origin) {
					return &scene.Expressions[i]
				}
			}
		}
	}
	return nil
}

func nextEtymologyReviewDate(logs []notebook.LearningRecord) string {
	if len(logs) == 0 || logs[0].IntervalDays == 0 {
		return ""
	}
	d := logs[0].LearnedAt.AddDate(0, 0, logs[0].IntervalDays)
	if !time.Now().Before(d) {
		return ""
	}
	return d.Format("2006-01-02")
}
