package main

import (
	"github.com/spf13/cobra"
)

// newQuizCommand groups the quiz-status helpers. The interactive quiz
// CLIs (`langner quiz freeform`, `langner quiz notebook`) were deleted
// once the web UI became the only sanctioned writer of user state —
// keeping a YAML-writing parallel path risked drift between the two
// stores.
func newQuizCommand() *cobra.Command {
	quizCommand := &cobra.Command{
		Use:   "quiz",
		Short: "Quiz commands for inspecting per-mode eligibility",
	}

	quizCommand.AddCommand(newQuizEtymologyStatusCommand())

	return quizCommand
}
