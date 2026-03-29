package quiz

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// createStoryFixtures creates story notebook fixtures in storiesDir with a learning file in learningDir.
func createStoryFixtures(t *testing.T, storiesDir, learningDir string) {
	t.Helper()
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
          quote: "That sounds preposterous to me."
      definitions:
        - expression: "preposterous"
          meaning: "contrary to reason or common sense"
`), 0644))
	// Write a learning history file so the notebook is recognised
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
`), 0644))
}

// createFlashcardFixtures creates flashcard notebook fixtures in flashcardsDir with a learning file in learningDir.
func createFlashcardFixtures(t *testing.T, flashcardsDir, learningDir string) {
	t.Helper()
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
      examples:
        - "It was pure serendipity that they met."
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "misunderstood"
          learned_at: "2025-01-14"
`), 0644))
}

func newTestService(t *testing.T, openaiClient inference.Client) *Service {
	t.Helper()
	learningDir := t.TempDir()
	return NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, openaiClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
}

func newTestServiceWithFixtures(t *testing.T, openaiClient inference.Client) (*Service, string) {
	t.Helper()
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()
	createStoryFixtures(t, storiesDir, learningDir)
	createFlashcardFixtures(t, flashcardsDir, learningDir)
	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, openaiClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return svc, learningDir
}

// ---------- LoadNotebookSummaries ----------

func TestService_LoadNotebookSummaries_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := newTestService(t, mock_inference.NewMockClient(ctrl))

	summaries, err := svc.LoadNotebookSummaries()
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestService_LoadNotebookSummaries_WithFixtures(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, _ := newTestServiceWithFixtures(t, mock_inference.NewMockClient(ctrl))

	summaries, err := svc.LoadNotebookSummaries()
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	summaryMap := make(map[string]NotebookSummary)
	for _, s := range summaries {
		summaryMap[s.NotebookID] = s
	}

	storySummary, ok := summaryMap["test-story"]
	require.True(t, ok)
	assert.Equal(t, "Test Story", storySummary.Name)
	// Only misunderstood answers in fixture → no words with correct answers → ReviewCount=0
	assert.Equal(t, 0, storySummary.ReviewCount)
	// Only misunderstood answers in fixture → no words eligible for reverse quiz
	assert.Equal(t, 0, storySummary.ReverseReviewCount)

	vocabSummary, ok := summaryMap["test-vocab"]
	require.True(t, ok)
	assert.Equal(t, "Test Vocabulary", vocabSummary.Name)
	assert.Equal(t, 1, vocabSummary.ReviewCount)
	assert.Equal(t, 0, vocabSummary.ReverseReviewCount)
}

func TestService_LoadNotebookSummaries_ReverseReviewCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	// Story with one word that has a correct answer (eligible for reverse)
	storyDir := filepath.Join(storiesDir, "my-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: my-story
name: My Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Episode 1"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Scene 1"
      conversations:
        - speaker: "Bob"
          quote: "He lost his temper completely."
      definitions:
        - expression: "lose one's temper"
          meaning: "to become very angry"
        - expression: "break the ice"
          meaning: "to initiate social interaction"
`), 0644))
	// Learning history: "lose one's temper" has a correct answer, "break the ice" does not
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "my-story.yml"), []byte(`- metadata:
    notebook_id: my-story
    title: "Episode 1"
  scenes:
    - metadata:
        title: "Scene 1"
      expressions:
        - expression: "lose one's temper"
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-14"
              interval_days: 1
        - expression: "break the ice"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
`), 0644))

	// Flashcard with one word that has a correct answer
	vocabDir := filepath.Join(flashcardsDir, "my-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: my-vocab
name: My Vocab
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Common Phrases"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
    - expression: "ephemeral"
      meaning: "lasting for a very short time"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "my-vocab.yml"), []byte(`- metadata:
    notebook_id: my-vocab
    title: "Common Phrases"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "understood"
          learned_at: "2025-01-14"
          interval_days: 1
    - expression: "ephemeral"
      learned_logs:
        - status: "misunderstood"
          learned_at: "2025-01-14"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	summaries, err := svc.LoadNotebookSummaries()
	require.NoError(t, err)

	summaryMap := make(map[string]NotebookSummary)
	for _, s := range summaries {
		summaryMap[s.NotebookID] = s
	}

	storySummary := summaryMap["my-story"]
	assert.Equal(t, 1, storySummary.ReviewCount, "only the word with a correct answer is counted")
	assert.Equal(t, 1, storySummary.ReverseReviewCount, "only the word with a correct answer is eligible for reverse")

	vocabSummary := summaryMap["my-vocab"]
	assert.Equal(t, 2, vocabSummary.ReviewCount)
	assert.Equal(t, 1, vocabSummary.ReverseReviewCount, "only the word with a correct answer is eligible for reverse")
}

func TestService_LoadNotebookSummaries_LearningHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "broken.yml"), []byte("{{invalid yaml"), 0644))

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	_, err := svc.LoadNotebookSummaries()
	require.Error(t, err)
}

// ---------- LoadCards ----------

func TestService_LoadCards_StoryNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()
	createStoryFixtures(t, storiesDir, learningDir)

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-story"}, true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "preposterous", cards[0].Entry)
	assert.Equal(t, "contrary to reason or common sense", cards[0].Meaning)
	assert.Equal(t, "test-story", cards[0].NotebookName)
	assert.Equal(t, "Chapter One", cards[0].StoryTitle)
	assert.Equal(t, "Opening", cards[0].SceneTitle)
	assert.NotEmpty(t, cards[0].Examples)
}

func TestService_LoadCards_FlashcardNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()
	createFlashcardFixtures(t, flashcardsDir, learningDir)

	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-vocab"}, true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "serendipity", cards[0].Entry)
	assert.Equal(t, "a fortunate discovery by accident", cards[0].Meaning)
	assert.Equal(t, "test-vocab", cards[0].NotebookName)
	assert.Equal(t, "flashcards", cards[0].StoryTitle)
	assert.Empty(t, cards[0].SceneTitle)
	require.Len(t, cards[0].Examples, 1)
	assert.Equal(t, "It was pure serendipity that they met.", cards[0].Examples[0].Text)
}

func TestService_LoadCards_MultipleNotebooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, _ := newTestServiceWithFixtures(t, mock_inference.NewMockClient(ctrl))

	cards, err := svc.LoadCards([]string{"test-story", "test-vocab"}, true)
	require.NoError(t, err)
	assert.Len(t, cards, 2)
}

func TestService_LoadCards_NotFoundError(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := newTestService(t, mock_inference.NewMockClient(ctrl))

	_, err := svc.LoadCards([]string{"non-existent"}, true)
	require.Error(t, err)
	var notFoundErr *NotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	assert.Equal(t, "non-existent", notFoundErr.NotebookID)
}

func TestService_LoadCards_DefinitionFieldUsedAsEntry(t *testing.T) {
	ctrl := gomock.NewController(t)
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
          quote: "He ran away quickly."
      definitions:
        - expression: "run"
          definition: "ran"
          meaning: "to move swiftly on foot"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-story"}, true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "ran", cards[0].Entry)
}

// ---------- GradeNotebookAnswer ----------

func TestService_GradeNotebookAnswer_Correct(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{
		Entry:   "preposterous",
		Meaning: "contrary to reason",
	}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "preposterous",
					Meaning:    "contrary to reason",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Reason: "Good answer", Quality: 4},
					},
				},
			},
		}, nil,
	)

	result, err := svc.GradeNotebookAnswer(context.Background(), card, "contrary to reason", 1000)
	require.NoError(t, err)
	assert.True(t, result.Correct)
	assert.Equal(t, "Good answer", result.Reason)
	assert.Equal(t, 4, result.Quality)
}

func TestService_GradeNotebookAnswer_Incorrect(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{
		Entry:   "serendipity",
		Meaning: "a fortunate discovery by accident",
	}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "serendipity",
					Meaning:    "a fortunate discovery by accident",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Reason: "Wrong meaning", Quality: 1},
					},
				},
			},
		}, nil,
	)

	result, err := svc.GradeNotebookAnswer(context.Background(), card, "wrong answer", 1000)
	require.NoError(t, err)
	assert.False(t, result.Correct)
	assert.Equal(t, "Wrong meaning", result.Reason)
	assert.Equal(t, 1, result.Quality)
}

func TestService_GradeNotebookAnswer_InferenceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{Entry: "preposterous"}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{}, assert.AnError,
	)

	_, err := svc.GradeNotebookAnswer(context.Background(), card, "some answer", 1000)
	require.Error(t, err)
}

func TestService_GradeNotebookAnswer_EmptyAnswers(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{Entry: "preposterous"}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{Answers: nil}, nil,
	)

	_, err := svc.GradeNotebookAnswer(context.Background(), card, "some answer", 1000)
	require.Error(t, err)
}

// ---------- SaveResult ----------

func TestService_SaveResult_WritesFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "test-vocab",
		StoryTitle:   "flashcards",
		SceneTitle:   "",
		Entry:        "serendipity",
		Meaning:      "a fortunate discovery by accident",
	}

	err := svc.SaveResult(context.Background(), card, GradeResult{Correct: true, Quality: 4}, 1000)
	require.NoError(t, err)

	historyPath := filepath.Join(learningDir, "test-vocab.yml")
	_, statErr := os.Stat(historyPath)
	assert.NoError(t, statErr)
}

func TestService_SaveResult_MalformedYAMLError(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-notebook.yml"), []byte("{{invalid yaml"), 0644))

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "test-notebook",
		StoryTitle:   "flashcards",
		Entry:        "preposterous",
	}

	err := svc.SaveResult(context.Background(), card, GradeResult{Correct: true, Quality: 4}, 1000)
	require.Error(t, err)
}

// ---------- helper functions (package-internal) ----------

func TestExtractAnswerResult(t *testing.T) {
	tests := []struct {
		name        string
		result      inference.AnswerMeaning
		wantCorrect bool
		wantReason  string
		wantQuality int
	}{
		{
			name:        "empty answers returns incorrect with quality 1",
			result:      inference.AnswerMeaning{AnswersForContext: nil},
			wantCorrect: false,
			wantReason:  "",
			wantQuality: 1,
		},
		{
			name: "correct answer extracts fields",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "Good answer", Quality: 5},
				},
			},
			wantCorrect: true,
			wantReason:  "Good answer",
			wantQuality: 5,
		},
		{
			name: "quality zero defaults to 4 for correct",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "OK", Quality: 0},
				},
			},
			wantCorrect: true,
			wantReason:  "OK",
			wantQuality: 4,
		},
		{
			name: "quality zero defaults to 1 for incorrect",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: false, Reason: "Wrong", Quality: 0},
				},
			},
			wantCorrect: false,
			wantReason:  "Wrong",
			wantQuality: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCorrect, gotReason, gotQuality := extractAnswerResult(tt.result)
			assert.Equal(t, tt.wantCorrect, gotCorrect)
			assert.Equal(t, tt.wantReason, gotReason)
			assert.Equal(t, tt.wantQuality, gotQuality)
		})
	}
}

func TestCountStoryDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		stories []notebook.StoryNotebook
		want    int
	}{
		{
			name:    "empty stories",
			stories: nil,
			want:    0,
		},
		{
			name: "counts definitions across stories and scenes",
			stories: []notebook.StoryNotebook{
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
						{Definitions: []notebook.Note{{Expression: "c"}}},
					},
				},
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "d"}}},
					},
				},
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countStoryDefinitions(tt.stories)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountFlashcardCards(t *testing.T) {
	tests := []struct {
		name      string
		notebooks []notebook.FlashcardNotebook
		want      int
	}{
		{
			name:      "empty notebooks",
			notebooks: nil,
			want:      0,
		},
		{
			name: "counts cards across notebooks",
			notebooks: []notebook.FlashcardNotebook{
				{Cards: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
				{Cards: []notebook.Note{{Expression: "c"}}},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countFlashcardCards(tt.notebooks)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildFromConversations(t *testing.T) {
	tests := []struct {
		name         string
		scene        notebook.StoryScene
		definition   notebook.Note
		wantExamples int
	}{
		{
			name: "skips empty quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: ""},
					{Speaker: "Bob", Quote: "This is absolutely preposterous."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 1,
		},
		{
			name: "skips non-matching quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "Hello there."},
					{Speaker: "Bob", Quote: "Good morning."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 0,
		},
		{
			name: "matches multiple quotes containing expression",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "That is preposterous!"},
					{Speaker: "Bob", Quote: "I agree, totally preposterous."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			examples, contexts := buildFromConversations(&tt.scene, &tt.definition)
			assert.Len(t, examples, tt.wantExamples)
			assert.Len(t, contexts, tt.wantExamples)
		})
	}
}

func TestContainsExpression(t *testing.T) {
	tests := []struct {
		name       string
		textLower  string
		expression string
		definition string
		want       bool
	}{
		{
			name:       "matches expression",
			textLower:  "i need to comprehend this",
			expression: "comprehend",
			definition: "",
			want:       true,
		},
		{
			name:       "matches definition",
			textLower:  "he ran away quickly",
			expression: "run",
			definition: "ran",
			want:       true,
		},
		{
			name:       "no match",
			textLower:  "hello world",
			expression: "comprehend",
			definition: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsExpression(tt.textLower, tt.expression, tt.definition)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindMatchingCards(t *testing.T) {
	cards := []FreeformCard{
		{
			Expression:         "jump down someone's throat",
			OriginalExpression: "jumped down my throat",
			Meaning:            "to speak sharply to someone",
		},
		{
			Expression:         "break the ice",
			OriginalExpression: "",
			Meaning:            "to initiate social interaction",
		},
		{
			Expression:         "lose one's temper",
			OriginalExpression: "lost her temper",
			Meaning:            "to become very angry",
		},
	}

	tests := []struct {
		name      string
		word      string
		wantCount int
		wantExpr  string
	}{
		{
			name:      "matches canonical expression",
			word:      "break the ice",
			wantCount: 1,
			wantExpr:  "break the ice",
		},
		{
			name:      "matches original expression from story text",
			word:      "jumped down my throat",
			wantCount: 1,
			wantExpr:  "jump down someone's throat",
		},
		{
			name:      "case insensitive match on canonical",
			word:      "Break The Ice",
			wantCount: 1,
			wantExpr:  "break the ice",
		},
		{
			name:      "case insensitive match on original",
			word:      "Lost Her Temper",
			wantCount: 1,
			wantExpr:  "lose one's temper",
		},
		{
			name:      "no match",
			word:      "spill the beans",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMatchingCards(cards, tt.word)
			assert.Len(t, got, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantExpr, got[0].Expression)
			}
		})
	}
}

func TestDeduplicateReverseCards(t *testing.T) {
	tests := []struct {
		name      string
		cards     []ReverseCard
		wantCount int
		validate  func(t *testing.T, result []ReverseCard)
	}{
		{
			name:      "empty input",
			cards:     []ReverseCard{},
			wantCount: 0,
		},
		{
			name: "no duplicates",
			cards: []ReverseCard{
				{Expression: "break the ice", Meaning: "to initiate conversation"},
				{Expression: "lose one's temper", Meaning: "to become very angry"},
			},
			wantCount: 2,
		},
		{
			name: "duplicates - keeps card with more contexts",
			cards: []ReverseCard{
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{{Context: "She broke the ice.", MaskedContext: "She ______ the ice."}}},
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []ReverseCard) {
				assert.Equal(t, 1, len(result[0].Contexts), "should keep card with more contexts")
			},
		},
		{
			name: "case insensitive dedup",
			cards: []ReverseCard{
				{Expression: "Break the Ice", Meaning: "to initiate conversation"},
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{{Context: "She broke the ice.", MaskedContext: "She ______ the ice."}}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []ReverseCard) {
				assert.Equal(t, 1, len(result[0].Contexts), "should keep card with more contexts")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateReverseCards(tt.cards)
			assert.Equal(t, tt.wantCount, len(result))
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestDeduplicateCards(t *testing.T) {
	tests := []struct {
		name      string
		cards     []Card
		wantCount int
		validate  func(t *testing.T, result []Card)
	}{
		{
			name:      "empty input",
			cards:     []Card{},
			wantCount: 0,
		},
		{
			name: "no duplicates",
			cards: []Card{
				{Entry: "break the ice", Meaning: "to initiate conversation"},
				{Entry: "lose one's temper", Meaning: "to become very angry"},
			},
			wantCount: 2,
		},
		{
			name: "duplicates - keeps card with more examples",
			cards: []Card{
				{Entry: "break the ice", Meaning: "to initiate conversation", Examples: []Example{{Text: "She broke the ice.", Speaker: "Alice"}}},
				{Entry: "break the ice", Meaning: "to initiate conversation"},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []Card) {
				assert.Equal(t, 1, len(result[0].Examples), "should keep card with more examples")
			},
		},
		{
			name: "case insensitive dedup",
			cards: []Card{
				{Entry: "Break the Ice", Meaning: "to initiate conversation"},
				{Entry: "break the ice", Meaning: "to initiate conversation", Examples: []Example{{Text: "She broke the ice.", Speaker: "Alice"}}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []Card) {
				assert.Equal(t, 1, len(result[0].Examples), "should keep card with more examples")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateCards(tt.cards)
			assert.Equal(t, tt.wantCount, len(result))
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
