package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

func newTestNotebookHandler(t *testing.T) *NotebookHandler {
	t.Helper()

	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()

	return NewNotebookHandler(
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningNotesDir,
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)
}

func newTestNotebookHandlerWithFixtures(t *testing.T) (*NotebookHandler, string) {
	t.Helper()

	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  metadata:
    series: "Test Series"
    season: 1
    episode: 1
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "That sounds {{ preposterous }} to me."
      definitions:
        - expression: "preposterous"
          meaning: "contrary to reason or common sense"
    - scene: "Closing"
      conversations:
        - speaker: "Bob"
          quote: "I find that {{ ludicrous }}."
      definitions:
        - expression: "ludicrous"
          meaning: "so foolish or unreasonable as to be amusing"
`), 0644))

	return NewNotebookHandler(
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningDir,
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	), learningDir
}

func TestNotebookHandler_GetNotebookDetail(t *testing.T) {
	tests := []struct {
		name     string
		req      *apiv1.GetNotebookDetailRequest
		wantCode connect.Code
		wantErr  bool
	}{
		{
			name:     "returns INVALID_ARGUMENT when notebook_id is empty",
			req:      &apiv1.GetNotebookDetailRequest{NotebookId: ""},
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns NOT_FOUND for non-existent notebook",
			req:      &apiv1.GetNotebookDetailRequest{NotebookId: "non-existent"},
			wantCode: connect.CodeNotFound,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestNotebookHandler(t)

			resp, err := handler.GetNotebookDetail(
				context.Background(),
				connect.NewRequest(tt.req),
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
				connectErr, ok := err.(*connect.Error)
				require.True(t, ok)
				assert.Equal(t, tt.wantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestNotebookHandler_GetNotebookDetail_WithFixtures(t *testing.T) {
	handler, _ := newTestNotebookHandlerWithFixtures(t)

	resp, err := handler.GetNotebookDetail(
		context.Background(),
		connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "test-story"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	msg := resp.Msg
	assert.Equal(t, "test-story", msg.GetNotebookId())
	assert.Equal(t, "Test Story", msg.GetName())
	assert.Equal(t, int32(2), msg.GetTotalWordCount())

	stories := msg.GetStories()
	require.Len(t, stories, 1)

	story := stories[0]
	assert.Equal(t, "Chapter One", story.GetEvent())
	assert.Equal(t, "2025-01-15", story.GetDate())

	metadata := story.GetMetadata()
	require.NotNil(t, metadata)
	assert.Equal(t, "Test Series", metadata.GetSeries())
	assert.Equal(t, int32(1), metadata.GetSeason())
	assert.Equal(t, int32(1), metadata.GetEpisode())

	scenes := story.GetScenes()
	require.Len(t, scenes, 2)

	scene := scenes[0]
	assert.Equal(t, "Opening", scene.GetTitle())

	conversations := scene.GetConversations()
	require.Len(t, conversations, 1)
	assert.Equal(t, "Alice", conversations[0].GetSpeaker())
	assert.Contains(t, conversations[0].GetQuote(), "preposterous")

	definitions := scene.GetDefinitions()
	require.Len(t, definitions, 1)
	assert.Equal(t, "preposterous", definitions[0].GetExpression())
	assert.Equal(t, "contrary to reason or common sense", definitions[0].GetMeaning())
}

func TestNotebookHandler_GetNotebookDetail_WithLearningHistory(t *testing.T) {
	handler, learningDir := newTestNotebookHandlerWithFixtures(t)

	// Create learning history
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          easiness_factor: 2.3
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-20"
              quality: 4
              response_time_ms: 1500
              quiz_type: "notebook"
              interval_days: 7
`), 0644))

	resp, err := handler.GetNotebookDetail(
		context.Background(),
		connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "test-story"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	scenes := resp.Msg.GetStories()[0].GetScenes()
	require.Len(t, scenes, 2)

	definitions := scenes[0].GetDefinitions()
	require.Len(t, definitions, 1)

	word := definitions[0]
	assert.Equal(t, "preposterous", word.GetExpression())
	assert.Equal(t, "understood", word.GetLearningStatus())
	assert.Equal(t, 2.3, word.GetEasinessFactor())
	assert.Equal(t, "2025-01-27", word.GetNextReviewDate())

	logs := word.GetLearnedLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, "understood", logs[0].GetStatus())
	assert.Equal(t, "2025-01-20", logs[0].GetLearnedAt())
	assert.Equal(t, int32(4), logs[0].GetQuality())
	assert.Equal(t, int64(1500), logs[0].GetResponseTimeMs())
	assert.Equal(t, "notebook", logs[0].GetQuizType())
	assert.Equal(t, int32(7), logs[0].GetIntervalDays())
}

func TestNotebookHandler_ExportNotebookPDF(t *testing.T) {
	tests := []struct {
		name     string
		req      *apiv1.ExportNotebookPDFRequest
		wantCode connect.Code
		wantErr  bool
	}{
		{
			name:     "returns INVALID_ARGUMENT when notebook_id is empty",
			req:      &apiv1.ExportNotebookPDFRequest{NotebookId: ""},
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns NOT_FOUND for non-existent notebook",
			req:      &apiv1.ExportNotebookPDFRequest{NotebookId: "non-existent"},
			wantCode: connect.CodeNotFound,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestNotebookHandler(t)

			resp, err := handler.ExportNotebookPDF(
				context.Background(),
				connect.NewRequest(tt.req),
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
				connectErr, ok := err.(*connect.Error)
				require.True(t, ok)
				assert.Equal(t, tt.wantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestNotebookHandler_ExportNotebookPDF_WithFixtures(t *testing.T) {
	handler, _ := newTestNotebookHandlerWithFixtures(t)

	resp, err := handler.ExportNotebookPDF(
		context.Background(),
		connect.NewRequest(&apiv1.ExportNotebookPDFRequest{NotebookId: "test-story"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	msg := resp.Msg
	assert.Equal(t, "test-story.pdf", msg.GetFilename())
	assert.NotEmpty(t, msg.GetPdfContent())
}

func TestNotebookHandler_GetNotebookDetail_LearningHistoryError(t *testing.T) {
	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "Hello there."
      definitions:
        - expression: "hello"
          meaning: "a greeting"
`), 0644))

	// Place a malformed YAML file in the learning directory
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "broken.yml"), []byte("{{invalid yaml"), 0644))

	handler := NewNotebookHandler(
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningDir,
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)

	resp, err := handler.GetNotebookDetail(
		context.Background(),
		connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "test-story"}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestNotebookHandler_LookupWord_FromCache(t *testing.T) {
	dictionaryMap := map[string]rapidapi.Response{
		"break": {
			Results: []rapidapi.Result{
				{PartOfSpeech: "verb", Definition: "to separate into pieces", Examples: []string{"she broke the ice"}},
			},
		},
	}

	handler := NewNotebookHandler(
		config.NotebooksConfig{},
		config.TemplatesConfig{},
		dictionaryMap,
		nil,
		nil,
		nil,
	)

	resp, err := handler.LookupWord(
		context.Background(),
		connect.NewRequest(&apiv1.LookupWordRequest{Word: "break"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "break", resp.Msg.GetWord())
	assert.Equal(t, "dictionary", resp.Msg.GetSource())
	require.Len(t, resp.Msg.GetDefinitions(), 1)
	assert.Equal(t, "verb", resp.Msg.GetDefinitions()[0].GetPartOfSpeech())
}

func TestNotebookHandler_LookupWord_NotFound(t *testing.T) {
	handler := NewNotebookHandler(
		config.NotebooksConfig{},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)

	resp, err := handler.LookupWord(
		context.Background(),
		connect.NewRequest(&apiv1.LookupWordRequest{Word: "unknownword"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "unknownword", resp.Msg.GetWord())
	assert.Empty(t, resp.Msg.GetDefinitions())
}

func TestNotebookHandler_RegisterDefinition(t *testing.T) {
	defsDir := t.TempDir()

	handler := NewNotebookHandler(
		config.NotebooksConfig{
			DefinitionsDirectories: []string{defsDir},
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)

	resp, err := handler.RegisterDefinition(
		context.Background(),
		connect.NewRequest(&apiv1.RegisterDefinitionRequest{
			NotebookId:   "mybook",
			NotebookFile: "001-chapter-1.yml",
			SceneIndex:   0,
			Expression:   "lose one's temper",
			Meaning:      "to become very angry",
			PartOfSpeech: "phrase",
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify the definitions file was written
	data, err := os.ReadFile(filepath.Join(defsDir, "mybook.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "lose one's temper")
	assert.Contains(t, string(data), "001-chapter-1.yml")
}

func TestNotebookHandler_RegisterDefinition_Subdirectory(t *testing.T) {
	defsDir := t.TempDir()

	handler := NewNotebookHandler(
		config.NotebooksConfig{
			DefinitionsDirectories: []string{defsDir},
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)

	resp, err := handler.RegisterDefinition(
		context.Background(),
		connect.NewRequest(&apiv1.RegisterDefinitionRequest{
			NotebookId:   "books/mybook",
			NotebookFile: "001-chapter-1.yml",
			SceneIndex:   0,
			Expression:   "break the ice",
			Meaning:      "to do something to relieve tension",
			PartOfSpeech: "phrase",
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify the file was created under the subdirectory, not at the top level
	data, err := os.ReadFile(filepath.Join(defsDir, "books", "mybook.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "break the ice")
}

func TestNotebookHandler_RegisterDefinition_PathTraversal(t *testing.T) {
	defsDir := t.TempDir()

	handler := NewNotebookHandler(
		config.NotebooksConfig{
			DefinitionsDirectories: []string{defsDir},
		},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		nil,
	)

	_, err := handler.RegisterDefinition(
		context.Background(),
		connect.NewRequest(&apiv1.RegisterDefinitionRequest{
			NotebookId: "../../etc/passwd",
			Expression: "test",
			Meaning:    "test meaning",
		}),
	)

	require.Error(t, err, "path traversal should be rejected")
}
