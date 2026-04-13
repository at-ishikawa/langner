package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/pdf"
)

// FlashcardNotebookWriter handles writing flashcard notebooks to various output formats
type FlashcardNotebookWriter struct {
	reader       *Reader
	templatePath string
}

func NewFlashcardNotebookWriter(reader *Reader, templatePath string) *FlashcardNotebookWriter {
	return &FlashcardNotebookWriter{
		reader:       reader,
		templatePath: templatePath,
	}
}

func (writer FlashcardNotebookWriter) OutputFlashcardNotebooks(
	flashcardID string,
	dictionaryMap map[string]rapidapi.Response,
	learningHistories map[string][]LearningHistory,
	sortDesc bool,
	outputDirectory string,
	generatePDF bool,
) error {
	notebooks, err := writer.reader.ReadFlashcardNotebooks(flashcardID)
	if err != nil {
		return fmt.Errorf("ReadFlashcardNotebooks() > %w", err)
	}
	if len(notebooks) == 0 {
		return fmt.Errorf("no flashcard notebooks found for %s", flashcardID)
	}
	learningHistory := learningHistories[flashcardID]

	notebooks, err = FilterFlashcardNotebooks(notebooks, learningHistory, dictionaryMap, sortDesc)
	if err != nil {
		return fmt.Errorf("FilterFlashcardNotebooks() > %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDirectory, 0755); err != nil {
		return fmt.Errorf("os.MkdirAll(%s) > %w", outputDirectory, err)
	}

	// Generate output filename from flashcard ID
	outputFilename := strings.TrimSpace(filepath.Join(outputDirectory, flashcardID+".md"))

	output, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("os.Create(%s) > %w", outputFilename, err)
	}
	defer func() {
		_ = output.Close()
	}()

	// Convert notebooks to assets format
	templateData := convertToAssetsFlashcardTemplate(notebooks)
	if err := assets.WriteFlashcardNotebook(output, writer.templatePath, templateData); err != nil {
		return fmt.Errorf("assets.WriteFlashcardNotebook(%s, %s, ) > %w", outputFilename, writer.templatePath, err)
	}

	fmt.Printf("Flashcard notebook written to: %s\n", outputFilename)

	if generatePDF {
		pdfPath, err := pdf.ConvertMarkdownToPDF(outputFilename)
		if err != nil {
			return fmt.Errorf("ConvertMarkdownToPDF(%s) > %w", outputFilename, err)
		}
		fmt.Printf("PDF generated at: %s\n", pdfPath)
	}

	return nil
}

// convertToAssetsFlashcardTemplate converts notebook types to assets.FlashcardTemplate for template rendering
func convertToAssetsFlashcardTemplate(notebooks []FlashcardNotebook) assets.FlashcardTemplate {
	assetsNotebooks := make([]assets.FlashcardNotebook, len(notebooks))
	for i, nb := range notebooks {
		assetsNotebooks[i] = convertFlashcardNotebook(nb)
	}
	return assets.FlashcardTemplate{
		Notebooks: assetsNotebooks,
	}
}

func convertFlashcardNotebook(nb FlashcardNotebook) assets.FlashcardNotebook {
	assetsCards := make([]assets.FlashcardCard, len(nb.Cards))
	for i, card := range nb.Cards {
		assetsCards[i] = assets.FlashcardCard{
			Expression:    card.Expression,
			Definition:    card.Definition,
			Meaning:       card.Meaning,
			Examples:      card.Examples,
			Pronunciation: card.Pronunciation,
			PartOfSpeech:  card.PartOfSpeech,
			Origin:        card.Origin,
			Synonyms:      card.Synonyms,
			Antonyms:      card.Antonyms,
			Images:        card.Images,
		}
	}
	return assets.FlashcardNotebook{
		Title:       nb.Title,
		Description: nb.Description,
		Date:        nb.Date,
		Cards:       assetsCards,
	}
}
