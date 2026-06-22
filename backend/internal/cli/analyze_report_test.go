package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestRunAnalyzeReport(t *testing.T) {
	tests := []struct {
		name       string
		histories  map[string][]notebook.LearningHistory
		year       int
		month      int
		wantErr    bool
	}{
		{
			name: "valid learning history",
			histories: map[string][]notebook.LearningHistory{
				"test-id": {{
					Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-id", Title: "Test Notebook"},
					Scenes: []notebook.LearningScene{{
						Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []notebook.LearningHistoryExpression{{
							Expression: "hello",
							LearnedLogs: []notebook.LearningRecord{{
								Status:       notebook.LearnedStatusUnderstood,
								LearnedAt:    notebook.Date{Time: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
								Quality:      4,
								IntervalDays: 3,
							}},
						}},
					}},
				}},
			},
			year:    2025,
			month:   6,
			wantErr: false,
		},
		{
			name:      "empty histories",
			histories: map[string][]notebook.LearningHistory{},
			year:      2025,
			month:     6,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunAnalyzeReport(tt.histories, tt.year, tt.month)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
