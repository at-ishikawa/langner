package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLearningHistoryUpdater_UpdateOrCreateExpression(t *testing.T) {
	tests := []struct {
		name            string
		initialHistory  []LearningHistory
		notebookID      string
		storyTitle      string
		sceneTitle      string
		expression      string
		isCorrect       bool
		isKnownWord     bool
		alwaysRecord    bool
		wantFound       bool
		wantExpressions int
		wantStatus      LearnedStatus
		wantLogs        int
	}{
		{
			name:            "Create new expression in empty history",
			initialHistory:  []LearningHistory{},
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			alwaysRecord:    false,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      LearnedStatusUnderstood,
		},
		{
			name: "Update existing expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{
								Title: "Scene 1",
							},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test-word",
									LearnedLogs: []LearningRecord{
										{
											Status:    LearnedStatusMisunderstood,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:     "test-notebook",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 1",
			expression:     "test-word",
			isCorrect:      true,
			isKnownWord:    true,
			alwaysRecord:   false,
			wantFound:    true,
			wantExpressions: 1,
			wantStatus: LearnedStatusUnderstood,
		},
		{
			name: "Create new scene in existing story",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{
								Title: "Scene 1",
							},
							Expressions: []LearningHistoryExpression{},
						},
					},
				},
			},
			notebookID:     "test-notebook",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 2",
			expression:     "test-word",
			isCorrect:      true,
			isKnownWord:    false,
			alwaysRecord:   false,
			wantFound:    false,
			wantExpressions: 1,
			wantStatus: LearnedStatusCanBeUsed,
		},
		{
			name: "Create new story in existing history",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{},
				},
			},
			notebookID:     "test-notebook",
			storyTitle:     "Story 2",
			sceneTitle:     "Scene 1",
			expression:     "test-word",
			isCorrect:      false,
			isKnownWord:    true,
			alwaysRecord:   false,
			wantFound:    false,
			wantExpressions: 1,
			wantStatus: LearnedStatusMisunderstood,
		},
		{
			name:           "Empty expression name",
			initialHistory: []LearningHistory{},
			notebookID:     "notebook1",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 1",
			expression:     "",
			isCorrect:      true,
			isKnownWord:    true,
			alwaysRecord:   false,
			wantFound:    false,
			wantExpressions: 1,
			wantStatus: LearnedStatusUnderstood,
		},
		{
			name:           "Special characters in names",
			initialHistory: []LearningHistory{},
			notebookID:     "notebook1",
			storyTitle:     "Story: With Special Characters!",
			sceneTitle:     "Scene (with parentheses)",
			expression:     "word/with/slashes",
			isCorrect:      true,
			isKnownWord:    true,
			alwaysRecord:   false,
			wantFound:    false,
			wantExpressions: 1,
			wantStatus: LearnedStatusUnderstood,
		},
		{
			name: "Update expression with existing logs",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{
								Title: "Scene 1",
							},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "word1",
									LearnedLogs: []LearningRecord{
										{
											Status:    LearnedStatusMisunderstood,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
										{
											Status:    LearnedStatusCanBeUsed,
											LearnedAt: Date{Time: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:     "notebook1",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 1",
			expression:     "word1",
			isCorrect:      true,
			isKnownWord:    true,
			alwaysRecord:   false,
			wantFound:    true,
			wantExpressions: 1,
			wantStatus: LearnedStatusUnderstood,
			wantLogs:   3,
		},
		{
			name: "Add expression to scene with existing expressions",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{
								Title: "Scene 1",
							},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "word1",
									LearnedLogs: []LearningRecord{
										{
											Status:    LearnedStatusUnderstood,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
								{
									Expression: "word2",
									LearnedLogs: []LearningRecord{
										{
											Status:    LearnedStatusCanBeUsed,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:     "notebook1",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 1",
			expression:     "word3",
			isCorrect:      false,
			isKnownWord:    false,
			alwaysRecord:   false,
			wantFound:    false,
			wantExpressions: 3,
			wantStatus: LearnedStatusMisunderstood,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create updater with initial history
			updater := NewLearningHistoryUpdater(tc.initialHistory)

			// Update or create expression
			found := updater.UpdateOrCreateExpression(
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.isCorrect,
				tc.isKnownWord,
				tc.alwaysRecord,
			)

			// Verify if expression was found
			assert.Equal(t, tc.wantFound, found)

			// Get updated history
			history := updater.GetHistory()

			// Find the expression and verify
			var gotExpression *LearningHistoryExpression
			var gotExpressions int

			for _, story := range history {
				if story.Metadata.Title == tc.storyTitle {
					for _, scene := range story.Scenes {
						if scene.Metadata.Title == tc.sceneTitle {
							gotExpressions = len(scene.Expressions)
							for _, exp := range scene.Expressions {
								if exp.Expression == tc.expression {
									gotExpression = &exp
									break
								}
							}
						}
					}
				}
			}

			require.NotNil(t, gotExpression, "Expression should exist in history")
			assert.Equal(t, tc.wantExpressions, gotExpressions, "Total expressions count mismatch")
			assert.Equal(t, tc.wantStatus, gotExpression.GetLatestStatus())

			if tc.wantLogs > 0 {
				assert.Len(t, gotExpression.LearnedLogs, tc.wantLogs)
			}
		})
	}
}
