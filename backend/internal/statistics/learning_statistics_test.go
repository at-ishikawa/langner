package statistics

import (
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
)

func TestCalculateStatistics(t *testing.T) {
	tests := []struct {
		name              string
		histories         map[string][]notebook.LearningHistory
		year              int
		month             int
		expectedPeriods   []LearningStatistics
		expectedAggregate AggregateStatistics
	}{
		{
			name: "single expression with one learn",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Test Notebook",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "hello",
										LearnedLogs: []notebook.LearningRecord{
											{
												LearnedAt: mustParseDate("2025-01-15"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "single expression with multiple reviews in same month",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "world",
										// Logs are stored newest first
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-20")}, // relearn
											{LearnedAt: mustParseDate("2025-01-18")}, // relearn
											{LearnedAt: mustParseDate("2025-01-15")}, // first learn (new word)
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  2,
					RelearnsUnique: 1, // Same word, so unique=1 even with 2 relearns
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  2,
				RelearnsUnique: 1,
			},
		},
		{
			name: "multiple expressions across different months",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "january",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
									{
										Expression: "february",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-02-10")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-02",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  2,
				NewWordsUnique: 2,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "filter by year",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "2024word",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2024-12-15")},
										},
									},
									{
										Expression: "2025word",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  2025,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "filter by year and month",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "january",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
									{
										Expression: "february",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-02-10")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  2025,
			month: 1,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "multiple scenes and notebooks",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word1",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
								},
							},
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word2",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-16")},
										},
									},
								},
							},
						},
					},
				},
				"notebook2": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word3",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-17")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  3,
					NewWordsUnique: 3,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  3,
				NewWordsUnique: 3,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "same word relearned across different periods - global unique counts",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-03-10")}, // relearn in March
											{LearnedAt: mustParseDate("2025-02-15")}, // relearn in February
											{LearnedAt: mustParseDate("2025-01-20")}, // first learn in January
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-03",
					NewWordsCount:  0,
					NewWordsUnique: 0,
					RelearnsCount:  1,
					RelearnsUnique: 1,
				},
				{
					Period:         "2025-02",
					NewWordsCount:  0,
					NewWordsUnique: 0,
					RelearnsCount:  1,
					RelearnsUnique: 1,
				},
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  2,
				RelearnsUnique: 1, // Same word relearned in Feb and Mar = 1 unique
			},
		},
		{
			name:      "empty histories",
			histories: map[string][]notebook.LearningHistory{},
			year:      0,
			month:     0,
			expectedPeriods: []LearningStatistics{},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  0,
				NewWordsUnique: 0,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "expression with no logs",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression:  "word",
										LearnedLogs: []notebook.LearningRecord{},
									},
								},
							},
						},
					},
				},
			},
			year:            0,
			month:           0,
			expectedPeriods: []LearningStatistics{},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  0,
				NewWordsUnique: 0,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "yearly aggregation without month filter",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word1",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
									{
										Expression: "word2",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-02-15")},
										},
									},
									{
										Expression: "word3",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2024-12-15")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  2025,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-02",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  2,
				NewWordsUnique: 2,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "misunderstood status only - should not count",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Test Notebook",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "difficult",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    notebook.LearnedStatusMisunderstood,
												LearnedAt: mustParseDate("2025-01-15"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			year:            0,
			month:           0,
			expectedPeriods: []LearningStatistics{},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  0,
				NewWordsUnique: 0,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "misunderstood followed by successful - counts as new word",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Test Notebook",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    notebook.LearnedStatusMisunderstood,
												LearnedAt: mustParseDate("2025-01-18"),
											},
											{
												LearnedAt: mustParseDate("2025-01-15"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "same expression learned multiple times in same period - unique vs total",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Test Notebook",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-25")}, // 3rd time (relearn)
											{LearnedAt: mustParseDate("2025-01-20")}, // 2nd time (relearn)
											{LearnedAt: mustParseDate("2025-01-15")}, // 1st time (new word)
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  1,
					NewWordsUnique: 1,
					RelearnsCount:  2,  // Total: 2 relearn events
					RelearnsUnique: 1,  // Unique: same word = 1
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  1,
				NewWordsUnique: 1,
				RelearnsCount:  2,
				RelearnsUnique: 1,
			},
		},
		{
			name: "different expressions with same word in different scenes - counts separately",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Notebook 1",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "hello",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-15")},
										},
									},
								},
							},
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 2",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "hello",
										LearnedLogs: []notebook.LearningRecord{
											{LearnedAt: mustParseDate("2025-01-16")},
										},
									},
								},
							},
						},
					},
				},
			},
			year:  0,
			month: 0,
			expectedPeriods: []LearningStatistics{
				{
					Period:         "2025-01",
					NewWordsCount:  2,
					NewWordsUnique: 2,
					RelearnsCount:  0,
					RelearnsUnique: 0,
				},
			},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  2,
				NewWordsUnique: 2,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
		{
			name: "zero date logs should be skipped",
			histories: map[string][]notebook.LearningHistory{
				"notebook1": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "nb1",
							Title:      "Test Notebook",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "word",
										LearnedLogs: []notebook.LearningRecord{
											{
												LearnedAt: notebook.Date{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			year:            0,
			month:           0,
			expectedPeriods: []LearningStatistics{},
			expectedAggregate: AggregateStatistics{
				NewWordsCount:  0,
				NewWordsUnique: 0,
				RelearnsCount:  0,
				RelearnsUnique: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateStatistics(tt.histories, tt.year, tt.month)
			assert.Equal(t, tt.expectedPeriods, result.Periods, "periods mismatch")
			assert.Equal(t, tt.expectedAggregate, result.Aggregate, "aggregate mismatch")
		})
	}
}

// mustParseDate is a helper function to create Date objects for testing
func mustParseDate(dateStr string) notebook.Date {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		panic(err)
	}
	return notebook.Date{Time: t}
}
