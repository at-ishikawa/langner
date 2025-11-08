# Architecture Documentation

## Overview

Langner is a Go application that manages vocabulary learning from various media sources (TV shows, games, books). It uses a three-layer data model architecture with separated concerns for YAML input, business logic, and output generation.

**Codebase Statistics:**
- Total Go files: 46
- Test files: 16
- Main packages: cmd/, internal/
- Key dependencies: Cobra (CLI), Viper (config), YAML v3 (data format)

## Core Architecture

### Three-Layer Data Model Architecture

The application follows a **separation of concerns** pattern with distinct layers for data handling:

```
YAML Files (Storage Format)
    ↓
YAML Models (Exact YAML structure)
    ↓
Output Models (Business logic & processing)
    ↓
Templates → Markdown → PDF/HTML
```

**Layer Details:**

1. **YAML Models** (`internal/notebook/`)
   - Direct representation of YAML file structure
   - Used for serialization/deserialization
   - Contains only field mappings and tags

2. **Output Models** (`internal/notebook/`)
   - Business logic and scoring algorithms
   - Learning progression calculations
   - Spaced repetition thresholds
   - Methods like:
     - `getLearnScore()`: Priority scoring for spaced repetition
     - `needsToLearnInFlashcard()`: Determines flashcard inclusion
     - `needsToLearnInStory()`: Determines story inclusion
     - `getNextLearningThresholdDays()`: Calculates next review interval

3. **Converters** (implicit in readers)
   - Dictionary integration (RapidAPI)
   - Learning history merging
   - Template data preparation

### Key Domain Models

```go
// Core vocabulary entry
type Note struct {
    Expression     string           // The word to learn
    Definition     string           // Alternative form
    Meaning        string           // Dictionary meaning
    Examples       []string         // Usage examples
    LearnedLogs    []LearningRecord // Learning history
    DictionaryNumber int            // Index into RapidAPI results
}

// Learning status progression
type LearnedStatus string
// Values: "" (learning) → "misunderstood" → "understood" → "usable" → "intuitive"

// Story structure for context-based learning
type StoryNotebook struct {
    Event    string       // Episode title
    Metadata Metadata     // Series/season/episode info
    Scenes   []StoryScene // Scenes containing conversations
}

// Scene with vocabulary definitions
type StoryScene struct {
    Title         string          // Scene name
    Conversations []Conversation  // Dialogue for context
    Definitions   []Note          // Vocabulary to learn
}

// Learning history tracking
type LearningHistory struct {
    Metadata LearningHistoryMetadata
    Scenes   []LearningScene
}
```

## Package Structure

### `/cmd` - Command Line Interface

Implements Cobra CLI commands. Each command file represents a top-level command:

**Command Hierarchy:**
```
langner
├── notebooks
│   └── stories <id>          # Generate stories with vocabulary
├── quiz
│   ├── freeform              # User inputs both word and meaning
│   └── notebook <name>       # Show word, user provides meaning
├── dictionary <word>         # Look up dictionary definitions
├── validate [--fix]          # Check data consistency
├── analyze [--days|--start|--end] # Learning statistics
└── parse <source> <file>     # Parse raw notebook files
```

**Key Patterns:**
- Each command loads configuration via `config.Load()`
- Creates necessary service objects (Reader, Validator, CLI)
- Delegates to internal packages for business logic
- Configuration file path via `--config` flag

### `/internal/notebook` - Core Domain Logic

Manages all vocabulary and learning data.

**Submodules:**

| File | Purpose |
|------|---------|
| `notebook.go` | Core `Note` model with learning logic |
| `flashcard.go` | Flashcard reader and filtering |
| `story.go` | Story notebook structures and rendering |
| `learning_history.go` | Learning record tracking |
| `learning_history_updater.go` | Updates learning progress |
| `file.go` | Generic YAML file I/O utilities |
| `validator.go` | Data validation logic |

**Data Flow:**

```
1. Reader loads YAML files from disk
   - `NewReader()` scans for index.yml files
   - Loads story notebooks from paths

2. Merges with dictionary data
   - Dictionary maps (RapidAPI responses) injected
   - `setDetails()` enriches Note with definition/examples

3. Merges with learning history
   - Learning history loaded from learning_notes_directory
   - `GetLogs()` matches expressions to history records

4. Filters based on spaced repetition algorithm
   - `needsToLearnInFlashcard()` checks if due for review
   - Thresholds: 1→7→30→90→365 days

5. Outputs via templates
   - Go templates render to Markdown
   - Marp CLI converts to PDF
```

### `/internal/cli` - Interactive User Sessions

Handles interactive quiz modes and user input.

**Architecture Pattern - Template Method:**

```go
// Base class with shared initialization
type InteractiveQuizCLI struct {
    learningNotesDir  string
    learningHistories map[string][]LearningHistory
    dictionaryMap     map[string]rapidapi.Response
    openaiClient      inference.Client
    stdinReader       *bufio.Reader
}

// Subclasses implement specific quiz logic
type FreeformQuizCLI struct { ... }
type NotebookQuizCLI struct { ... }

// Shared run loop with signal handling
func (cli *InteractiveQuizCLI) Run(ctx context.Context, session Session) error
```

**Key Types:**

- `InteractiveQuizCLI`: Base for interactive sessions
- `Session` interface: Implement quiz-specific logic
- `FreeformQuizCLI`: User provides both word and meaning
- `NotebookQuizCLI`: Application shows word, user provides meaning

**Features:**
- Graceful shutdown on Ctrl+C
- Color-coded terminal output (bold/italic)
- Real-time learning history updates
- Buffered stdin reading for interactive input

### `/internal/dictionary` - Word Definitions

Manages dictionary lookups and caching.

**Architecture:**

```go
// Reader handles lookup and caching
type Reader struct {
    config    Config          // API credentials
    fileCache *FileCache      // Local JSON cache
}

// FileCache pattern
func (r *Reader) Lookup(ctx context.Context, word string) Response {
    contents, err := r.fileCache.cache(word, func() ([]byte, error) {
        return r.lookupAPI(ctx, word)  // Call API if not cached
    })
}
```

**Data Integration Points:**
- Called from `Note.setDetails()` to enrich vocabulary
- Results mapped by expression: `map[string]rapidapi.Response`
- Dictionary number (1-indexed) selects from results array
- Caches RapidAPI JSON responses locally in `dictionaries/rapidapi/`

**RapidAPI Response Structure:**
```go
type Response struct {
    Results      []Definition
    Pronunciation PronunciationInfo
}

type Definition struct {
    PartOfSpeech string
    Definition   string
    Examples     []string
    Synonyms     []string
}
```

### `/internal/inference` - AI Integration

Provides interface for AI-powered learning features.

**Interface Design:**

```go
type Client interface {
    AnswerExpressionWithSingleContext(ctx, params) AnswerQuestionResponse
    AnswerExpressionWithMultipleContexts(ctx, params) MultipleAnswerQuestionResponse
}
```

**Implementations:**
- OpenAI client for GPT-4o-mini (default)
- Retry logic with exponential backoff
- Context-based answer validation

**Used by:**
- `FreeformQuizCLI`: Validates user input against meaning
- `NotebookQuizCLI`: Validates if answer matches expected meaning
- Mock client available for testing

### `/internal/config` - Configuration Management

Uses Viper for hierarchical configuration.

**Configuration Hierarchy (precedence):**
1. Command-line flags (`--config path`)
2. Environment variables (RapidAPI, OpenAI keys)
3. File: `config.yml` (current dir) or `config.yaml`
4. File: `~/.langner/config.yaml`
5. Built-in defaults

**Configuration Structure:**
```yaml
notebooks:
  stories_directory: notebooks/stories
  learning_notes_directory: notebooks/learning_notes
dictionaries:
  rapidapi:
    cache_directory: dictionaries/rapidapi
templates:
  markdown_directory: assets/templates
outputs:
  story_directory: outputs/story
openai:
  model: gpt-4o-mini
```

**Security Notes:**
- API credentials (RapidAPI, OpenAI) loaded **only from environment variables**
- Never stored in config files
- Environment bindings: `RAPID_API_KEY`, `OPENAI_API_KEY`, `OPENAI_MODEL`

### `/internal/mocks` - Testing

Generated mock implementations using `mockgen`.

**Current Mocks:**
- `mock_inference.Client`: For testing quiz CLIs without API calls

## Data Flow Patterns

### Pattern 1: Story Notebook Generation

```
1. NewReader(storiesDir, dictionaryMap)
   ↓
2. ReadStoryNotebooks(storyID)
   - Scans index.yml files
   - Loads YAML files from notebook paths
   ↓
3. NewLearningHistories(learningNotesDir)
   - Loads all learning_notes/*.yml files
   - Indexed by notebook name
   ↓
4. FilterStoryNotebooks()
   - For each note: merge with learning history
   - Check if needs to learn (via thresholds)
   - Merge with dictionary data
   ↓
5. ConvertStoryNotebookMarkers()
   - Converts {{ expression }} markers
   - Markdown format for template rendering
   ↓
6. Template execution
   - Renders to story-marp.md.go.tmpl
   - Outputs to story_directory/*.md
```

### Pattern 2: Quiz Session

```
1. NewFreeformQuizCLI() or NewNotebookQuizCLI()
   - Load dictionary
   - Load learning histories
   - Initialize OpenAI client
   ↓
2. Load quiz cards (words to practice)
   - From notebooks
   - Filtered by learning status
   - Sorted by priority
   ↓
3. Interactive loop:
   - Present question
   - Read user answer
   - Call OpenAI to validate
   - Update learning history
   - Save to YAML file
   ↓
4. Graceful shutdown
   - Ctrl+C signal handling
   - Final learning history saved
```

## Spaced Repetition Algorithm

The core learning logic implements spaced repetition with status-based thresholds:

**Status Progression:**
```
New word → Learning → Misunderstood ↔ Understood → Usable → Intuitive
```

**Threshold Rules:**
```go
thresholds := map[int]int{
    1: 7,      // Show again in 7 days after first "understood"
    2: 30,     // Then 30 days
    3: 90,     // Then 90 days
    4: 365,    // Then 365 days
}

// After 4+ successful reviews, show in 1000 days
if count > 4 { return 1000 }
```

**Scoring Formula:**
```
score = baseScore - daysSinceLastReview - daysSinceNotebookCreation

// baseScore weights:
// misunderstood: -5
// understood: +10
// usable: +1000
// intuitive: +100000
```

**Filtering Rules:**

*For Flashcards:*
- Always include: misunderstood expressions
- Include if: now > (lastReviewDate + threshold)
- Exclude if: threshold > lowerThresholdDay (default 30 days)

*For Stories:*
- Always include: misunderstood expressions
- Include if: now > (lastReviewDate + threshold) AND threshold <= 1
- Exclude once learned and past 7-day threshold

## File Organization Patterns

### Notebook Directory Structure

```
notebooks/
├── stories/                  # Story notebooks
│   ├── series-id/
│   │   ├── index.yml        # References to season files
│   │   ├── season01.yml     # Story entries (array)
│   │   └── season02.yml
│   └── another-series/
│       └── index.yml
│
└── learning_notes/          # Learning history
    ├── series-id.yml        # Tracks progress (array)
    └── another-series.yml

dictionaries/
└── rapidapi/               # Cached API responses (JSON)
    ├── word1.json
    └── word2.json

outputs/
└── story/                  # Generated outputs
    ├── series-id.md
    └── another-series.md

assets/
└── templates/
    └── story-marp.md.go.tmpl  # Go template for rendering
```

### YAML Index File Format

```yaml
id: "series-id"
kind: "TVShows"
name: "Display Name"
notebooks:
  - "./season01.yml"
  - "./season02.yml"
```

## Testing Strategy

**Test Organization:**
- Tests in corresponding `*_test.go` files
- 16 test files covering core logic
- Table-driven tests for functions

**Key Test Patterns:**
- Use `assert` and `require` from testify
- Mock OpenAI client for CLI tests
- Validate YAML unmarshaling edge cases

**Testing Coverage Areas:**
- Spaced repetition calculations
- Learning history merging
- Data validation
- Marker conversion in story text
- Configuration loading

## External Dependencies

### Direct Dependencies
| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Configuration management |
| `gopkg.in/yaml.v3` | YAML parsing |
| `github.com/go-resty/resty/v2` | HTTP client for RapidAPI |
| `github.com/fatih/color` | Terminal color output |
| `github.com/stretchr/testify` | Testing assertions |
| `go.uber.org/mock` | Mock generation (mockgen) |

### External Services
- **RapidAPI WordsAPI**: Dictionary definitions
- **OpenAI GPT-4o-mini**: Quiz validation and grading

## Design Patterns Used

| Pattern | Location | Purpose |
|---------|----------|---------|
| Factory | `NewReader()`, `NewValidator()` | Create configured service objects |
| Strategy | `ConversionStyle` (ConversionStyleMarkdown, etc.) | Multiple output formats |
| Template Method | `InteractiveQuizCLI.Run()` | Shared quiz session logic |
| Decorator/Wrapper | `FileCache` | Add caching to dictionary lookup |
| Repository | `Reader`, `LearningHistories` | Abstraction over file system |
| Fluent Builder | Cobra command registration | Readable CLI definition |

## Code Organization Principles

Based on CLAUDE.md:

1. **Simplicity First**
   - Avoid over-engineering
   - Prefer early returns/continues
   - Minimize nesting

2. **No Util Packages**
   - Generic utilities avoided
   - Functions stay in domain packages
   - `xslices/` only for cross-cutting helpers

3. **Consistent Patterns**
   - Follow existing architecture style
   - Don't create new patterns
   - Consolidate related code

4. **Test-Driven Development**
   - Tests in corresponding `_test.go` files
   - Validate existing functionality
   - Use table-driven tests

5. **No Default Value Assignment**
   - Explicit initialization preferred
   - Zero values used naturally

## Future Architecture Considerations

**Potential Improvements:**
- Database storage (SQLite) for learning history
- Web UI instead of CLI
- Mobile app for on-the-go practice
- Plugin system for different question types
- Batch processing for large datasets
- Kubernetes deployment
