package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
)

// InteractiveQuizCLI contains shared logic for interactive quiz CLIs
type InteractiveQuizCLI struct {
	learningNotesDir  string
	learningHistories map[string][]notebook.LearningHistory
	dictionaryMap     map[string]rapidapi.Response
	openaiClient      inference.Client
	stdinReader       *bufio.Reader
	stdoutWriter      io.Writer
	bold              *color.Color
	italic            *color.Color
}

// newInteractiveQuizCLI creates the base CLI with shared initialization
func newInteractiveQuizCLI(
	storiesDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*InteractiveQuizCLI, *notebook.Reader, error) {
	// Load dictionary responses
	response, err := rapidapi.NewReader().Read(dictionaryCacheDir)
	if err != nil {
		return nil, nil, fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
	}
	dictionaryMap := rapidapi.FromResponsesToMap(response)

	// Create notebook reader
	reader, err := notebook.NewReader(storiesDir, "", dictionaryMap)
	if err != nil {
		return nil, nil, fmt.Errorf("notebook.NewReader() > %w", err)
	}

	// Load learning histories
	learningHistories, err := notebook.NewLearningHistories(learningNotesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("notebook.NewLearningHistories() > %w", err)
	}

	return &InteractiveQuizCLI{
		learningNotesDir:  learningNotesDir,
		learningHistories: learningHistories,
		dictionaryMap:     dictionaryMap,
		openaiClient:      openaiClient,
		stdinReader:       bufio.NewReader(os.Stdin),
		stdoutWriter:      os.Stdout,
		bold:              color.New(color.Bold),
		italic:            color.New(color.Italic),
	}, reader, nil
}

//go:generate mockgen -source=interactive_quiz_cli.go -destination=../mocks/cli/mock_session.go -package=mock_cli Session

type Session interface {
	Session(context context.Context) error
}

func (cli *InteractiveQuizCLI) Run(ctx context.Context, session Session) error {
	ctx, cancel := signal.NotifyContext(
		ctx,
		os.Interrupt,
	)
	defer cancel()

	errCh := make(chan error)
	go func() {
		defer close(errCh)

	LOOP:
		for {
			select {
			case <-ctx.Done():
				break LOOP
			default:
			}

			if err := session.Session(ctx); err != nil {
				if errors.Is(err, errEnd) {
					break
				}
				errCh <- err
				break
			}
		}
	}()
	select {
	case <-ctx.Done():
		fmt.Println("Received interrupt signal, exiting...")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error: %w", err)
		}
	}
	return nil
}

// updateLearningHistoryRecord updates or creates an expression in the learning history
func (cli *InteractiveQuizCLI) updateLearningHistory(
	notebookName string,
	learningHistory []notebook.LearningHistory,
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord, alwaysRecord bool,
) ([]notebook.LearningHistory, error) {
	updater := notebook.NewLearningHistoryUpdater(learningHistory)
	updater.UpdateOrCreateExpression(
		notebookID,
		storyTitle,
		sceneTitle,
		expression,
		isCorrect,
		isKnownWord,
		alwaysRecord,
	)
	learningHistory = updater.GetHistory()

	// Save learning history
	notePath := filepath.Join(cli.learningNotesDir, notebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, learningHistory); err != nil {
		return learningHistory, fmt.Errorf("failed to write a file %s > %w", notePath, err)
	}
	return learningHistory, nil
}
