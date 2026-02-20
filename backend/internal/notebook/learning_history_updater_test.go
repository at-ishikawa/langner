package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no change needed",
			input: "Scene Title",
			want:  "Scene Title",
		},
		{
			name:  "leading and trailing whitespace",
			input: "  Scene Title  ",
			want:  "Scene Title",
		},
		{
			name:  "multiple internal spaces",
			input: "Scene   Title",
			want:  "Scene Title",
		},
		{
			name:  "newlines and tabs",
			input: "Scene\n\tTitle",
			want:  "Scene Title",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLearningHistoryUpdater_UpdateOrCreateExpressionWithQualityForReverse(t *testing.T) {
	tests := []struct {
		name            string
		initialHistory  []LearningHistory
		notebookID      string
		storyTitle      string
		sceneTitle      string
		expression      string
		isCorrect       bool
		isKnownWord     bool
		quality         int
		responseTimeMs  int64
		wantFound       bool
	}{
		{
			name:            "Create new reverse expression in empty history",
			initialHistory:  []LearningHistory{},
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  5000,
			wantFound:       false,
		},
		{
			name: "Update existing reverse expression in scene",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression:            "test-word",
									LearnedLogs:           []LearningRecord{},
									ReverseEasinessFactor: DefaultEasinessFactor,
								},
							},
						},
					},
				},
			},
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "test-word",
			isCorrect:       false,
			isKnownWord:     false,
			quality:         int(QualityWrong),
			responseTimeMs:  10000,
			wantFound:       true,
		},
		{
			name: "Update existing flashcard reverse expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
					},
					Expressions: []LearningHistoryExpression{
						{
							Expression:            "test-word",
							LearnedLogs:           []LearningRecord{},
							ReverseEasinessFactor: DefaultEasinessFactor,
						},
					},
				},
			},
			notebookID:      "test-notebook",
			storyTitle:      "flashcards",
			sceneTitle:      "",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  3000,
			wantFound:       true,
		},
		{
			name:            "Create new flashcard reverse expression",
			initialHistory:  []LearningHistory{},
			notebookID:      "test-notebook",
			storyTitle:      "flashcards",
			sceneTitle:      "",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     false,
			quality:         int(QualityCorrectSlow),
			responseTimeMs:  6000,
			wantFound:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := NewLearningHistoryUpdater(tc.initialHistory)

			found := updater.UpdateOrCreateExpressionWithQualityForReverse(
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.isCorrect,
				tc.isKnownWord,
				tc.quality,
				tc.responseTimeMs,
			)

			assert.Equal(t, tc.wantFound, found)

			history := updater.GetHistory()
			require.NotEmpty(t, history)

			// Find the expression and verify reverse logs exist
			var gotExpression *LearningHistoryExpression
			for _, story := range history {
				if story.Metadata.Title == tc.storyTitle {
					if story.Metadata.Type == "flashcard" || (tc.storyTitle == "flashcards" && tc.sceneTitle == "") {
						for _, exp := range story.Expressions {
							if exp.Expression == tc.expression {
								gotExpression = &exp
								break
							}
						}
					} else {
						for _, scene := range story.Scenes {
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

			require.NotNil(t, gotExpression)
			assert.NotEmpty(t, gotExpression.ReverseLogs)
		})
	}
}

func TestLearningHistoryUpdater_UpdateOrCreateExpressionWithQuality(t *testing.T) {
	tests := []struct {
		name            string
		initialHistory  []LearningHistory
		notebookID      string
		storyTitle      string
		sceneTitle      string
		expression      string
		isCorrect       bool
		isKnownWord     bool
		quality         int
		responseTimeMs  int64
		quizType        QuizType
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
			quality:         int(QualityCorrect),
			responseTimeMs:  5000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
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
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  3000,
			quizType:        QuizTypeNotebook,
			wantFound:       true,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
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
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 2",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     false,
			quality:         int(QualityCorrect),
			responseTimeMs:  4000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      learnedStatusCanBeUsed,
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
			notebookID:      "test-notebook",
			storyTitle:      "Story 2",
			sceneTitle:      "Scene 1",
			expression:      "test-word",
			isCorrect:       false,
			isKnownWord:     true,
			quality:         int(QualityWrong),
			responseTimeMs:  10000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      LearnedStatusMisunderstood,
		},
		{
			name:            "Empty expression name",
			initialHistory:  []LearningHistory{},
			notebookID:      "notebook1",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  2000,
			quizType:        QuizTypeNotebook,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
		},
		{
			name:            "Special characters in names",
			initialHistory:  []LearningHistory{},
			notebookID:      "notebook1",
			storyTitle:      "Story: With Special Characters!",
			sceneTitle:      "Scene (with parentheses)",
			expression:      "word/with/slashes",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrectFast),
			responseTimeMs:  1000,
			quizType:        QuizTypeNotebook,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
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
											Status:    learnedStatusCanBeUsed,
											LearnedAt: Date{Time: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:      "notebook1",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "word1",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  5000,
			quizType:        QuizTypeFreeform,
			wantFound:       true,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
			wantLogs:        3,
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
											Status:    learnedStatusUnderstood,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
								{
									Expression: "word2",
									LearnedLogs: []LearningRecord{
										{
											Status:    learnedStatusCanBeUsed,
											LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:      "notebook1",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "word3",
			isCorrect:       false,
			isKnownWord:     false,
			quality:         int(QualityWrong),
			responseTimeMs:  15000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 3,
			wantStatus:      LearnedStatusMisunderstood,
		},
		{
			name: "Update existing expression with empty learned_logs",
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
									Expression:  "run some ideas by someone",
									LearnedLogs: []LearningRecord{},
								},
							},
						},
					},
				},
			},
			notebookID:      "notebook1",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene 1",
			expression:      "run some ideas by someone",
			isCorrect:       true,
			isKnownWord:     false,
			quality:         int(QualityCorrectSlow),
			responseTimeMs:  8000,
			quizType:        QuizTypeFreeform,
			wantFound:       true,
			wantExpressions: 1,
			wantStatus:      learnedStatusCanBeUsed,
			wantLogs:        1,
		},
		{
			name:            "Create new flashcard expression in empty history",
			initialHistory:  []LearningHistory{},
			notebookID:      "test-notebook",
			storyTitle:      "flashcards",
			sceneTitle:      "",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  3000,
			quizType:        QuizTypeNotebook,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
		},
		{
			name: "Update existing flashcard expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
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
			notebookID:      "test-notebook",
			storyTitle:      "flashcards",
			sceneTitle:      "",
			expression:      "test-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  4000,
			quizType:        QuizTypeNotebook,
			wantFound:       true,
			wantExpressions: 1,
			wantStatus:      learnedStatusUnderstood,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := NewLearningHistoryUpdater(tc.initialHistory)

			found := updater.UpdateOrCreateExpressionWithQuality(
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.isCorrect,
				tc.isKnownWord,
				tc.quality,
				tc.responseTimeMs,
				tc.quizType,
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
					// For flashcard type, check expressions directly
					if story.Metadata.Type == "flashcard" || (tc.storyTitle == "flashcards" && tc.sceneTitle == "") {
						gotExpressions = len(story.Expressions)
						for _, exp := range story.Expressions {
							if exp.Expression == tc.expression {
								gotExpression = &exp
								break
							}
						}
					} else {
						// For story type, check scenes
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
