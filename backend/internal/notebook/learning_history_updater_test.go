package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeQuotes(t *testing.T) {
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
			name:  "smart single quotes to ASCII",
			input: "it\u2019s time",
			want:  "it's time",
		},
		{
			name:  "smart double quotes to ASCII",
			input: "\u201CHello\u201D",
			want:  "\"Hello\"",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeQuotes(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLearningHistoryUpdater_UpdateOrCreateExpressionWithQualityForReverse(t *testing.T) {
	tests := []struct {
		name                    string
		initialHistory          []LearningHistory
		notebookID              string
		storyTitle              string
		sceneTitle              string
		expression              string
		originalExpression      string
		isCorrect               bool
		isKnownWord             bool
		quality                 int
		responseTimeMs          int64
		wantFound               bool
		wantExpressionsLen      int // if > 0, assert flashcard expressions len
		wantScenesLen           int // if > 0, assert scenes len
		wantSceneExpressionsLen int // if > 0, assert first scene's expressions len
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
		{
			name: "flashcard type matches but expression not found creates new expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
					},
					Expressions: []LearningHistoryExpression{
						{
							Expression:            "existing-word",
							LearnedLogs:           []LearningRecord{},
							},
					},
				},
			},
			notebookID:         "test-notebook",
			storyTitle:         "flashcards",
			sceneTitle:         "",
			expression:         "new-word",
			isCorrect:          true,
			isKnownWord:        true,
			quality:            int(QualityCorrect),
			responseTimeMs:     5000,
			wantFound:          false,
			wantExpressionsLen: 2,
		},
		{
			name: "scene not found creates new scene",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene A"},
							Expressions: []LearningHistoryExpression{
								{
									Expression:            "word-a",
									LearnedLogs:           []LearningRecord{},
								},
							},
						},
					},
				},
			},
			notebookID:     "test-notebook",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene B",
			expression:     "word-b",
			isCorrect:      true,
			isKnownWord:    false,
			quality:        int(QualityCorrect),
			responseTimeMs: 5000,
			wantFound:      false,
			wantScenesLen:  2,
		},
		{
			name: "expression not found in existing scene creates new expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene A"},
							Expressions: []LearningHistoryExpression{
								{
									Expression:            "word-a",
									LearnedLogs:           []LearningRecord{},
								},
							},
						},
					},
				},
			},
			notebookID:              "test-notebook",
			storyTitle:              "Story 1",
			sceneTitle:              "Scene A",
			expression:              "word-b",
			isCorrect:               true,
			isKnownWord:             true,
			quality:                 int(QualityCorrect),
			responseTimeMs:          5000,
			wantFound:               false,
			wantSceneExpressionsLen: 2,
		},
		{
			name: "finds existing entry by original expression when definition is used as lookup key",
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
									Expression:            "break the ice",
									LearnedLogs:           []LearningRecord{{Status: LearnedStatusUnderstood, LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}},
																	},
							},
						},
					},
				},
			},
			notebookID:         "test-notebook",
			storyTitle:         "Story 1",
			sceneTitle:         "Scene 1",
			expression:         "ease the tension",
			originalExpression: "break the ice",
			isCorrect:          true,
			isKnownWord:        true,
			quality:            int(QualityCorrect),
			responseTimeMs:     5000,
			wantFound:          true,
		},
		{
			name: "finds existing flashcard entry by original expression when definition is used as lookup key",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
					},
					Expressions: []LearningHistoryExpression{
						{
							Expression:            "lose one's temper",
							LearnedLogs:           []LearningRecord{{Status: LearnedStatusUnderstood, LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}},
														},
					},
				},
			},
			notebookID:         "test-notebook",
			storyTitle:         "flashcards",
			sceneTitle:         "",
			expression:         "become very angry",
			originalExpression: "lose one's temper",
			isCorrect:          true,
			isKnownWord:        true,
			quality:            int(QualityCorrect),
			responseTimeMs:     3000,
			wantFound:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := NewLearningHistoryUpdater(tc.initialHistory, nil)

			found := updater.UpdateOrCreateExpressionWithQualityForReverse(
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.originalExpression,
				tc.isCorrect,
				tc.isKnownWord,
				tc.quality,
				tc.responseTimeMs,
				QuizTypeReverse,
			)

			assert.Equal(t, tc.wantFound, found)

			history := updater.GetHistory()
			require.NotEmpty(t, history)

			// Find the expression and verify reverse logs exist
			var gotExpression *LearningHistoryExpression
			matchExpression := func(expName string) bool {
				if expName == tc.expression {
					return true
				}
				if tc.originalExpression != "" && expName == tc.originalExpression {
					return true
				}
				return false
			}
			for _, story := range history {
				if story.Metadata.Title == tc.storyTitle {
					if story.Metadata.Type == "flashcard" || (tc.storyTitle == "flashcards" && tc.sceneTitle == "") {
						for _, exp := range story.Expressions {
							if matchExpression(exp.Expression) {
								gotExpression = &exp
								break
							}
						}
					} else {
						for _, scene := range story.Scenes {
							for _, exp := range scene.Expressions {
								if matchExpression(exp.Expression) {
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

			if tc.wantExpressionsLen > 0 {
				for _, story := range history {
					if story.Metadata.Title == tc.storyTitle {
						assert.Len(t, story.Expressions, tc.wantExpressionsLen)
					}
				}
			}
			if tc.wantScenesLen > 0 {
				for _, story := range history {
					if story.Metadata.Title == tc.storyTitle {
						assert.Len(t, story.Scenes, tc.wantScenesLen)
					}
				}
			}
			if tc.wantSceneExpressionsLen > 0 {
				for _, story := range history {
					if story.Metadata.Title == tc.storyTitle {
						require.NotEmpty(t, story.Scenes)
						assert.Len(t, story.Scenes[0].Expressions, tc.wantSceneExpressionsLen)
					}
				}
			}
		})
	}
}

func TestLearningHistoryUpdater_UpdateOrCreateExpressionWithQuality(t *testing.T) {
	tests := []struct {
		name               string
		initialHistory     []LearningHistory
		notebookID         string
		storyTitle         string
		sceneTitle         string
		expression         string
		originalExpression string
		isCorrect          bool
		isKnownWord        bool
		quality            int
		responseTimeMs     int64
		quizType           QuizType
		wantFound          bool
		wantExpressions    int
		wantStatus         LearnedStatus
		wantLogs           int
		wantScenesLen      int // if > 0, assert the number of scenes
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
			wantStatus:      LearnedStatusUnderstood,
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
			wantStatus:      LearnedStatusCanBeUsed,
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
			wantStatus:      LearnedStatusUnderstood,
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
			wantStatus:      LearnedStatusUnderstood,
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
			wantStatus:      LearnedStatusUnderstood,
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
			wantStatus:      LearnedStatusCanBeUsed,
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
			wantStatus:      LearnedStatusUnderstood,
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
			wantStatus:      LearnedStatusUnderstood,
		},
		{
			name: "flashcard type matches but expression not found creates new expression",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
					},
					Expressions: []LearningHistoryExpression{
						{
							Expression:  "existing-word",
							LearnedLogs: []LearningRecord{},
						},
					},
				},
			},
			notebookID:      "test-notebook",
			storyTitle:      "flashcards",
			sceneTitle:      "",
			expression:      "new-word",
			isCorrect:       true,
			isKnownWord:     true,
			quality:         int(QualityCorrect),
			responseTimeMs:  5000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 2,
			wantStatus:      LearnedStatusUnderstood,
		},
		{
			name: "story matches but scene not found creates new scene",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene A"},
							Expressions: []LearningHistoryExpression{
								{Expression: "word-a", LearnedLogs: []LearningRecord{}},
							},
						},
					},
				},
			},
			notebookID:      "test-notebook",
			storyTitle:      "Story 1",
			sceneTitle:      "Scene B",
			expression:      "word-b",
			isCorrect:       true,
			isKnownWord:     false,
			quality:         int(QualityCorrect),
			responseTimeMs:  5000,
			quizType:        QuizTypeFreeform,
			wantFound:       false,
			wantExpressions: 1,
			wantStatus:      LearnedStatusCanBeUsed,
			wantScenesLen:   2,
		},
		{
			name: "finds existing entry by original expression when definition is used as lookup key",
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
									Expression:     "break the ice",
									LearnedLogs:    []LearningRecord{{Status: LearnedStatusUnderstood, LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}},
																	},
							},
						},
					},
				},
			},
			notebookID:         "test-notebook",
			storyTitle:         "Story 1",
			sceneTitle:         "Scene 1",
			expression:         "ease the tension",
			originalExpression: "break the ice",
			isCorrect:          true,
			isKnownWord:        true,
			quality:            int(QualityCorrect),
			responseTimeMs:     5000,
			quizType:           QuizTypeNotebook,
			wantFound:          true,
			wantExpressions:    1,
			wantStatus:         LearnedStatusUnderstood,
			wantLogs:           2,
		},
		{
			name: "finds existing flashcard entry by original expression when definition is used as lookup key",
			initialHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "flashcards",
						Type:       "flashcard",
					},
					Expressions: []LearningHistoryExpression{
						{
							Expression:     "lose one's temper",
							LearnedLogs:    []LearningRecord{{Status: LearnedStatusUnderstood, LearnedAt: Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}},
													},
					},
				},
			},
			notebookID:         "test-notebook",
			storyTitle:         "flashcards",
			sceneTitle:         "",
			expression:         "become very angry",
			originalExpression: "lose one's temper",
			isCorrect:          true,
			isKnownWord:        true,
			quality:            int(QualityCorrect),
			responseTimeMs:     3000,
			quizType:           QuizTypeNotebook,
			wantFound:          true,
			wantExpressions:    1,
			wantStatus:         LearnedStatusUnderstood,
			wantLogs:           2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updater := NewLearningHistoryUpdater(tc.initialHistory, nil)

			found := updater.UpdateOrCreateExpressionWithQuality(
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.originalExpression,
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

			matchExpression := func(expName string) bool {
				if expName == tc.expression {
					return true
				}
				if tc.originalExpression != "" && expName == tc.originalExpression {
					return true
				}
				return false
			}

			for _, story := range history {
				if story.Metadata.Title == tc.storyTitle {
					// For flashcard type, check expressions directly
					if story.Metadata.Type == "flashcard" || (tc.storyTitle == "flashcards" && tc.sceneTitle == "") {
						gotExpressions = len(story.Expressions)
						for _, exp := range story.Expressions {
							if matchExpression(exp.Expression) {
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
									if matchExpression(exp.Expression) {
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
			if tc.wantScenesLen > 0 {
				for _, story := range history {
					if story.Metadata.Title == tc.storyTitle {
						assert.Len(t, story.Scenes, tc.wantScenesLen)
					}
				}
			}
		})
	}
}

// TestAssertNoDuplicateOriginsInSession_PassesAndFails verifies the
// pre-write invariant guard used by SaveEtymologyOriginResult. The
// happy-path returns nil; a duplicate origin across scenes (the bug
// class this guard catches) returns an error naming the offending
// origin and the scenes it landed in.
func TestAssertNoDuplicateOriginsInSession_PassesAndFails(t *testing.T) {
	clean := []LearningHistory{{
		Metadata: LearningHistoryMetadata{Title: "Session X"},
		Scenes: []LearningScene{{
			Metadata: LearningSceneMetadata{Title: "alpha (first)"},
			Expressions: []LearningHistoryExpression{{
				Expression: "demo-root",
				Type:       LearningExpressionTypeOrigin,
				EtymologyBreakdownLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Now()), Quality: 4, QuizType: "etymology_breakdown"},
				},
			}},
		}},
	}}
	assert.NoError(t, AssertNoDuplicateOriginsInSession(clean, "demo-notebook", "Session X"),
		"clean state must pass the guard")

	dirty := []LearningHistory{{
		Metadata: LearningHistoryMetadata{Title: "Session X"},
		Scenes: []LearningScene{
			{
				Metadata: LearningSceneMetadata{Title: "alpha (first)"},
				Expressions: []LearningHistoryExpression{{
					Expression: "demo-root",
					Type:       LearningExpressionTypeOrigin,
				}},
			},
			{
				Metadata: LearningSceneMetadata{Title: "beta (drifted)"},
				Expressions: []LearningHistoryExpression{{
					Expression: "demo-root",
					Type:       LearningExpressionTypeOrigin,
				}},
			},
		},
	}}
	err := AssertNoDuplicateOriginsInSession(dirty, "demo-notebook", "Session X")
	require.Error(t, err, "duplicate origin across scenes must trip the guard")
	assert.Contains(t, err.Error(), "demo-root", "error must name the offending origin")
	assert.Contains(t, err.Error(), "alpha (first)", "error must list both scenes")
	assert.Contains(t, err.Error(), "beta (drifted)", "error must list both scenes")
}

// TestUpdateOrCreateExpressionForEtymology_WritesToExistingScene pins
// the rule that stops "two logos sessions" from happening: when an
// etymology origin already lives under one scene in a session, a write
// arriving with a DIFFERENT scene title must update the existing entry
// in place (not create a duplicate under the new scene). Without this,
// any shift in pickBestSceneForOrigin's output — from a definitions
// edit, the determinism fix, or anything else that changes the
// candidate list — splits the origin's learning history.
func TestUpdateOrCreateExpressionForEtymology_WritesToExistingScene(t *testing.T) {
	// Generic Greek-root pair, not from the user's data.
	history := []LearningHistory{{
		Metadata: LearningHistoryMetadata{Title: "Session X"},
		Scenes: []LearningScene{{
			Metadata: LearningSceneMetadata{Title: "alpha (first)"},
			Expressions: []LearningHistoryExpression{{
				Expression: "demo-root",
				Type:       LearningExpressionTypeOrigin,
				EtymologyBreakdownLogs: []LearningRecord{
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-24 * time.Hour)), Quality: 4, QuizType: "etymology_freeform"},
				},
			}},
		}},
	}}

	updater := NewLearningHistoryUpdater(history, nil)
	// Write arrives addressed to a DIFFERENT scene title — what
	// pickBestSceneForOrigin would now produce after a candidate-set
	// drift. The lookup must still find the existing entry under
	// "alpha (first)" and update it there.
	found := updater.UpdateOrCreateExpressionWithQualityForEtymology(
		"demo-notebook", "Session X", "beta (drifted)",
		"demo-root", "", true, true, 5, 2000,
		QuizTypeEtymologyStandard,
	)
	assert.True(t, found, "must find the existing origin under its current scene, not create a duplicate")

	got := updater.GetHistory()
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1, "no new scene must be created on a same-session origin write")
	assert.Equal(t, "alpha (first)", got[0].Scenes[0].Metadata.Title,
		"the existing entry must stay under its original scene title")
	require.Len(t, got[0].Scenes[0].Expressions, 1)
	exp := got[0].Scenes[0].Expressions[0]
	assert.Equal(t, "demo-root", exp.Expression)
	assert.Len(t, exp.EtymologyBreakdownLogs, 2,
		"the new log must be appended onto the existing entry's logs")
}
