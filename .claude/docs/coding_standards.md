# Coding Standards

This document outlines the coding standards and best practices for the Langner application. These standards are derived from the project's CLAUDE.md and observed patterns in the codebase.

## Core Principles

### 1. Simplicity First
- Implement code as simple as possible
- Avoid over-engineering and unnecessary abstractions
- Prefer straightforward solutions over clever ones
- Keep functions focused and small

### 2. Early Returns and Minimal Nesting
- Avoid deep nesting in `for`, `if`, and `else` blocks
- Use `continue` or `return` to exit early
- Prefer guard clauses over nested conditionals

**Good Example:**
```go
func processNote(note Note) error {
    if note.Expression == "" {
        return errors.New("expression is required")
    }
    if note.Meaning == "" {
        return errors.New("meaning is required")
    }

    // Process note...
    return nil
}
```

**Bad Example:**
```go
func processNote(note Note) error {
    if note.Expression != "" {
        if note.Meaning != "" {
            // Process note...
            return nil
        } else {
            return errors.New("meaning is required")
        }
    } else {
        return errors.New("expression is required")
    }
}
```

### 3. No Backward Compatibility in Internal Packages
- Ignore backward compatibility in `cmd` and `internal` packages
- This allows for cleaner refactoring and simplification
- Feel free to change internal APIs as needed

### 4. No Default Value Assignment
- Don't explicitly assign zero/default values to variables
- Rely on Go's zero value initialization
- Only assign non-zero values explicitly

**Good Example:**
```go
var count int        // Implicitly 0
var name string      // Implicitly ""
var enabled bool     // Implicitly false
```

**Bad Example:**
```go
var count int = 0
var name string = ""
var enabled bool = false
```

### 5. No Generic Utility Packages
- Don't create packages like `util`, `common`, or `helpers`
- Keep functions in their domain packages
- The `xslices/` package is an exception for cross-cutting slice utilities

## Package Organization

### Internal Package Structure

The codebase follows a domain-driven package structure under `internal/`:

```
internal/
├── notebook/      # Core vocabulary and learning domain
├── cli/          # Interactive quiz sessions
├── dictionary/   # Word definitions and caching
├── config/       # Configuration management
├── inference/    # AI integration interfaces
└── mocks/        # Generated mock implementations
```

### Package Naming
- Use singular nouns for package names (`notebook`, not `notebooks`)
- Keep package names short and descriptive
- Package names should match directory names

### Package Dependencies
- Avoid circular dependencies between packages
- `cmd` can depend on all `internal` packages
- `internal` packages should have minimal dependencies on each other
- Use interfaces to break dependencies when needed

## Code Structure

### File Organization

1. **One file per major type or concept**
   - `notebook.go` - Core Note and Notebook types
   - `flashcard.go` - Flashcard-specific logic
   - `story.go` - Story notebook structures
   - `validator.go` - Validation logic

2. **Test files next to source files**
   - `notebook.go` → `notebook_test.go`
   - Tests for a file must be in the corresponding `_test.go` file
   - Don't create separate test directories

3. **File size guidelines**
   - Keep files under 500 lines when possible
   - Split into logical submodules if files grow too large

### Type Definitions

#### Struct Definitions

```go
// Use descriptive struct names
type Note struct {
    Expression string `yaml:"expression,omitempty"`
    Meaning    string `yaml:"meaning,omitempty"`
    Examples   []string `yaml:"examples,omitempty"`
}

// Group related fields together
type Notebook struct {
    // Metadata
    Series string `yaml:"series,omitempty"`
    Source Source `yaml:"source,omitempty"`

    // Content
    Notes []Note `yaml:"notes,omitempty"`
    Date  time.Time `yaml:"date,omitempty"`
}
```

#### Enums and Constants

Use typed constants with const blocks:

```go
type LearnedStatus string

const (
    learnedStatusLearning        LearnedStatus = ""
    LearnedStatusMisunderstood   LearnedStatus = "misunderstood"
    learnedStatusUnderstood      LearnedStatus = "understood"
    learnedStatusCanBeUsed       LearnedStatus = "usable"
    learnedStatusIntuitivelyUsed LearnedStatus = "intuitive"
)
```

**Naming conventions:**
- Exported constants: `LearnedStatusMisunderstood`
- Unexported constants: `learnedStatusLearning`
- Use PascalCase for exported, camelCase for unexported

### Function Definitions

#### Function Naming

```go
// Exported functions use PascalCase
func NewReader(storiesDir string, dictionaryMap map[string]rapidapi.Response) (*Reader, error)

// Unexported functions use camelCase
func getLearnScore() int

// Boolean getters use "is" or "has" prefix (unexported)
func needsToLearnInFlashcard(lowerThresholdDay int) bool
```

#### Function Parameters

- Order parameters from general to specific
- Group related parameters into structs if there are more than 3-4 parameters
- Use context as the first parameter for functions that may do I/O

```go
// Good - few parameters
func processNote(expression, meaning string) error

// Good - many parameters grouped in struct
type Config struct {
    StoriesDir       string
    LearningNotesDir string
    DictionaryDir    string
}

func NewReader(cfg Config) (*Reader, error)

// Good - context first
func (r *Reader) Lookup(ctx context.Context, word string) (Response, error)
```

#### Return Values

- Return errors as the last return value
- Use named return values sparingly, only when they improve clarity
- Prefer explicit returns over naked returns

```go
// Preferred
func loadConfig(path string) (*Config, error) {
    cfg, err := parse(path)
    if err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    return cfg, nil
}

// Named returns only when helpful
func validateNote(note Note) (isValid bool, err error) {
    if note.Expression == "" {
        return false, errors.New("expression required")
    }
    return true, nil
}
```

### Error Handling

#### Error Wrapping

Always wrap errors with context using `fmt.Errorf` with `%w`:

```go
func ReadStoryNotebooks(id string) ([]StoryNotebook, error) {
    files, err := os.ReadDir(dir)
    if err != nil {
        return nil, fmt.Errorf("os.ReadDir() > %w", err)
    }

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("os.ReadFile(%s) > %w", path, err)
    }

    return notebooks, nil
}
```

**Error wrapping format:**
- Use `>` to show call chain: `"FunctionName() > %w"`
- Include relevant parameters when helpful: `"functionName(%s) > %w", param, err`
- Keep error messages lowercase (except for function names)

#### Error Types

Define custom error types for domain-specific errors:

```go
var (
    errEnd = errors.New("end of session")
    ErrInvalidExpression = errors.New("invalid expression")
)
```

## Testing Standards

### Test File Organization

1. **Tests in corresponding files**
   - `foo.go` → `foo_test.go`
   - All tests for functions in `foo.go` must be in `foo_test.go`

2. **Add to existing test functions**
   - Add test cases to existing test functions instead of creating new ones
   - Only create new test functions for new features or distinct test scenarios

### Table-Driven Tests

Use table-driven tests for all functions with multiple test cases:

```go
func TestNote_needsToLearnInFlashcard(t *testing.T) {
    baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

    tests := []struct {
        name              string
        note              Note
        lowerThresholdDay int
        want              bool
    }{
        {
            name: "no logs - doesn't need learning",
            note: Note{
                Expression: "hello",
                Definition: "greeting",
            },
            lowerThresholdDay: 0,
            want:              false,
        },
        {
            name: "old misunderstood - needs learning",
            note: Note{
                Expression: "hello",
                LearnedLogs: []LearningRecord{
                    {Status: LearnedStatusMisunderstood},
                },
            },
            lowerThresholdDay: 0,
            want:              true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.note.needsToLearnInFlashcard(tt.lowerThresholdDay)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Test Naming Conventions

- **Variable names:** Use `want` and `got` (not `expected` and `actual`)
- **Test case names:** Describe the scenario and expected outcome
- **Test function names:** `TestTypeName_MethodName` or `TestFunctionName`

**Examples:**
```go
func TestNote_getLearnScore(t *testing.T)
func TestNote_needsToLearnInFlashcard(t *testing.T)
func TestNewReader(t *testing.T)
```

### Assertions

Use `testify/assert` and `testify/require`:

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Use assert for non-critical checks
assert.Equal(t, want, got)
assert.NotNil(t, result)
assert.Greater(t, score, 0)

// Use require for critical checks that should stop the test
require.NoError(t, err)
require.NotNil(t, config)
```

**When to use each:**
- `require`: When the test cannot continue if the assertion fails (e.g., nil pointer would cause panic)
- `assert`: For all other assertions

### Test Organization

```go
func TestFunctionName(t *testing.T) {
    // 1. Setup test data
    baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

    // 2. Define test cases
    tests := []struct {
        name string
        args argStruct
        want resultType
    }{
        // test cases...
    }

    // 3. Run tests
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := functionName(tt.args)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## Code Style

### Imports

Group imports into three sections:
1. Standard library
2. External dependencies
3. Internal packages

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/spf13/cobra"
    "github.com/stretchr/testify/assert"
    "gopkg.in/yaml.v3"

    "github.com/at-ishikawa/langner/internal/config"
    "github.com/at-ishikawa/langner/internal/notebook"
)
```

### Comments

#### Package Comments

Every package should have a package comment:

```go
// Package notebook provides core vocabulary and learning management.
package notebook
```

#### Function Comments

- Comment exported functions and types
- Follow the format: "FunctionName does X"
- Don't comment obvious functions

```go
// NewReader creates a new notebook reader with the given configuration.
// It loads dictionary mappings and prepares the reader for loading notebooks.
func NewReader(storiesDir string, dictionaryMap map[string]rapidapi.Response) (*Reader, error)

// getLearnScore calculates a priority score for spaced repetition.
// Higher scores indicate higher priority for review.
func (n Note) getLearnScore() int
```

#### Inline Comments

Use inline comments for complex logic:

```go
// Threshold rules for spaced repetition
thresholds := map[int]int{
    1: 7,      // Show again in 7 days after first "understood"
    2: 30,     // Then 30 days
    3: 90,     // Then 90 days
    4: 365,    // Then 365 days
}
```

### Variable Naming

- Use short names for local variables with small scope: `i`, `j`, `err`, `cfg`
- Use descriptive names for package-level variables and struct fields
- Avoid abbreviations unless widely understood

```go
// Good local variables
for i, note := range notes {
    if err := validate(note); err != nil {
        return err
    }
}

// Good struct fields
type Config struct {
    StoriesDirectory       string
    LearningNotesDirectory string
}

// Avoid unclear abbreviations
// Bad: mnDir, lnDir, cfg
// Good: meaningDir, learningNotesDir, config
```

## Command-Line Interface

### Cobra Command Structure

```go
func newCommandName() *cobra.Command {
    var flagVar string

    cmd := &cobra.Command{
        Use:   "command-name",
        Short: "Brief description",
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(configFile)
            if err != nil {
                return err
            }

            // Command logic...
            return nil
        },
    }

    cmd.Flags().StringVar(&flagVar, "flag-name", "", "flag description")
    return cmd
}
```

### Flag Naming

- Use kebab-case for flag names: `--config-file`, `--learning-notes`
- Use single-letter shortcuts sparingly: `-c`, `-v`
- Provide helpful descriptions

## Development Workflow

### Pre-commit Checks

**Always run before committing:**

```bash
make pre-commit
```

This runs:
1. `golangci-lint run --fix` - Linting with auto-fixes
2. `go run ./cmd validate --fix` - Data validation with auto-fixes
3. `go test ./...` - All tests

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/notebook

# With coverage
go test -cover ./...

# Integration tests (requires OPENAI_API_KEY)
make test/integration
```

### Running Commands

Use `go run` instead of `go build`:

```bash
# Good
go run ./cmd notebooks flashcards

# Avoid
go build -o evb ./cmd
./evb notebooks flashcards
```

## Configuration Management

### Loading Configuration

```go
cfg, err := config.Load(configFile)
if err != nil {
    return fmt.Errorf("config.Load(): %w", err)
}
```

### Environment Variables

**API credentials must come from environment variables only:**

```go
// These are bound to environment variables in config.Load()
RAPID_API_HOST
RAPID_API_KEY
OPENAI_API_KEY
OPENAI_MODEL
```

**Never:**
- Store API keys in config files
- Commit API keys to version control
- Hard-code API keys in source code

## Dependencies

### Adding Dependencies

1. Only add dependencies when necessary
2. Prefer standard library when possible
3. Choose well-maintained packages with good documentation

```bash
go get github.com/package/name
go mod tidy
```

### Current Key Dependencies

- CLI: `github.com/spf13/cobra`
- Config: `github.com/spf13/viper`
- YAML: `gopkg.in/yaml.v3`
- HTTP: `github.com/go-resty/resty/v2`
- Testing: `github.com/stretchr/testify`
- Mocks: `go.uber.org/mock`
- Color: `github.com/fatih/color`

## Refactoring Guidelines

### When to Refactor

1. **Remove unused code immediately**
   - If dependencies exist only in test cases, remove the functions
   - Migrate test cases to new implementations

2. **Consolidate similar code**
   - Extract common patterns into shared functions
   - Don't create new architectural patterns

3. **Simplify complex functions**
   - Break down functions over 50 lines
   - Extract helper functions when logic is complex

### Refactoring Process

1. Write tests for existing behavior
2. Make the change
3. Run `make pre-commit` to ensure nothing breaks
4. Update tests to cover new functionality

## Common Patterns

### Factory Pattern

```go
func NewReader(storiesDir string, dictionaryMap map[string]rapidapi.Response) (*Reader, error) {
    if storiesDir == "" {
        return nil, errors.New("storiesDir is required")
    }

    return &Reader{
        storiesDir:    storiesDir,
        dictionaryMap: dictionaryMap,
    }, nil
}
```

### Template Method Pattern

```go
// Base struct with shared logic
type InteractiveQuizCLI struct {
    learningNotesDir string
    openaiClient     inference.Client
}

// Interface for customization
type Session interface {
    session(ctx context.Context) error
}

// Shared run loop
func (cli *InteractiveQuizCLI) Run(ctx context.Context, session Session) error {
    // Common initialization...
    return session.session(ctx)
}
```

### Repository Pattern

```go
type Reader struct {
    storiesDir    string
    dictionaryMap map[string]rapidapi.Response
}

func (r *Reader) ReadStoryNotebooks(id string) ([]StoryNotebook, error) {
    // Load from file system...
}
```

## Anti-Patterns to Avoid

### 1. God Objects
Don't create objects that do too much. Keep responsibilities focused.

### 2. Premature Optimization
Write clear code first, optimize only when there's a proven performance issue.

### 3. Deep Inheritance
Go doesn't have inheritance. Use composition and interfaces instead.

### 4. Ignoring Errors
Always handle errors, even if it's just logging them.

```go
// Bad
result, _ := doSomething()

// Good
result, err := doSomething()
if err != nil {
    return fmt.Errorf("doSomething: %w", err)
}
```

### 5. Mutable Global State
Avoid package-level variables that change. Use dependency injection instead.

## Performance Considerations

### When Performance Matters

1. **Loading large datasets** - Use streaming or pagination
2. **Repeated operations** - Cache results when appropriate
3. **File I/O** - Read files once and reuse data

### Caching Pattern

```go
type FileCache struct {
    cacheDir string
}

func (c *FileCache) cache(key string, fetchFunc func() ([]byte, error)) ([]byte, error) {
    // Check cache first
    if data, err := os.ReadFile(c.path(key)); err == nil {
        return data, nil
    }

    // Fetch and cache
    data, err := fetchFunc()
    if err != nil {
        return nil, err
    }

    if err := os.WriteFile(c.path(key), data, 0644); err != nil {
        return nil, err
    }

    return data, nil
}
```

## Security Best Practices

### 1. API Credentials
- Load from environment variables only
- Never commit to version control
- Use different credentials for different environments

### 2. File Permissions
- Use appropriate permissions: `0644` for files, `0755` for directories
- Validate file paths to prevent directory traversal

### 3. Input Validation
- Validate all external input (files, user input, API responses)
- Use the validator package for data validation

### 4. Error Messages
- Don't expose sensitive information in error messages
- Log detailed errors, show generic messages to users

## Documentation

### When to Document

1. All exported functions and types
2. Complex algorithms or business logic
3. Non-obvious behavior or edge cases
4. Configuration requirements

### README Updates

Keep README.md current with:
- New commands
- Configuration changes
- Architecture changes
- Dependencies updates

Run this after significant changes:
```bash
# Ensure documentation is current
git diff README.md
```

## Summary Checklist

Before submitting code, verify:

- [ ] Code follows simplicity principle
- [ ] Early returns used instead of deep nesting
- [ ] No default values assigned to variables
- [ ] Tests use table-driven approach with `want`/`got`
- [ ] Tests added to existing test functions when possible
- [ ] Errors wrapped with context using `%w`
- [ ] No API credentials in code or config files
- [ ] Comments added for exported functions
- [ ] `make pre-commit` passes successfully
- [ ] README.md updated if needed
