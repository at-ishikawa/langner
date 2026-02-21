package config

import (
	"fmt"
	"path/filepath"
	"strings"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port int        `mapstructure:"port"`
	CORS CORSConfig `mapstructure:"cors"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Notebooks    NotebooksConfig    `mapstructure:"notebooks"`
	Dictionaries DictionariesConfig `mapstructure:"dictionaries"`
	Templates    TemplatesConfig    `mapstructure:"templates"`
	Outputs      OutputsConfig      `mapstructure:"outputs"`
	OpenAI       OpenAIConfig       `mapstructure:"openai"`
	Books        BooksConfig        `mapstructure:"books"`
	Database     DatabaseConfig     `mapstructure:"database"`
}

type DatabaseConfig struct {
	Host            string            `mapstructure:"host"`
	Port            int               `mapstructure:"port"`
	Database        string            `mapstructure:"database"`
	Username        string            `mapstructure:"username"`
	Password        string            `mapstructure:"password"`
	TLS             bool              `mapstructure:"tls"`
	Params          map[string]string `mapstructure:"params"`
	MaxOpenConns    int               `mapstructure:"max_open_conns"`
	MaxIdleConns    int               `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int               `mapstructure:"conn_max_lifetime_seconds"`
}

type NotebooksConfig struct {
	StoriesDirectories     []string `mapstructure:"stories_directories"`
	LearningNotesDirectory string   `mapstructure:"learning_notes_directory"`
	FlashcardsDirectories  []string `mapstructure:"flashcards_directories"`
	BooksDirectories       []string `mapstructure:"books_directories"`
	DefinitionsDirectories []string `mapstructure:"definitions_directories"`
}

type TemplatesConfig struct {
	StoryNotebookTemplate     string `mapstructure:"story_notebook_template" validate:"omitempty,file"`
	FlashcardNotebookTemplate string `mapstructure:"flashcard_notebook_template" validate:"omitempty,file"`
}

type OutputsConfig struct {
	StoryDirectory     string `mapstructure:"story_directory"`
	FlashcardDirectory string `mapstructure:"flashcard_directory"`
}

type DictionariesConfig struct {
	RapidAPI RapidAPIConfig `mapstructure:"rapidapi"`
}

type RapidAPIConfig struct {
	CacheDirectory string `mapstructure:"cache_directory"`
	Host           string `mapstructure:"host"`
	Key            string `mapstructure:"key"`
}

type OpenAIConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type BooksConfig struct {
	RepoDirectory    string `mapstructure:"repo_directory"`
	RepositoriesFile string `mapstructure:"repositories_file"`
}

type ConfigLoader struct {
	viper      *viper.Viper
	validator  *validator.Validate
	translator ut.Translator
}

func NewConfigLoader(configFile string) (*ConfigLoader, error) {
	validate, trans, err := newValidator()
	if err != nil {
		return nil, fmt.Errorf("failed to create new validator: %w", err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/langner")
	}

	return &ConfigLoader{
		viper:      v,
		validator:  validate,
		translator: trans,
	}, nil
}

func (loader *ConfigLoader) Load() (*Config, error) {
	v := loader.viper

	v.SetDefault("notebooks.stories_directories", []string{filepath.Join("notebooks", "stories")})
	v.SetDefault("notebooks.learning_notes_directory", filepath.Join("notebooks", "learning_notes"))
	v.SetDefault("notebooks.flashcards_directories", []string{filepath.Join("notebooks", "flashcards")})
	v.SetDefault("dictionaries.rapidapi.cache_directory", filepath.Join("dictionaries", "rapidapi"))
	// Template is optional - if not specified, will use embedded fallback template
	v.SetDefault("templates.story_notebook_template", "")
	v.SetDefault("templates.flashcard_notebook_template", "")
	v.SetDefault("outputs.story_directory", filepath.Join("outputs", "story"))
	v.SetDefault("outputs.flashcard_directory", filepath.Join("outputs", "flashcard"))
	v.SetDefault("openai.model", "gpt-4o-mini")
	v.SetDefault("notebooks.books_directories", []string{filepath.Join("notebooks", "books")})
	v.SetDefault("notebooks.definitions_directories", []string{filepath.Join("notebooks", "definitions")})
	v.SetDefault("books.repo_directory", "ebooks")
	v.SetDefault("books.repositories_file", "books.yml")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 3306)
	v.SetDefault("database.database", "local")
	v.SetDefault("database.username", "user")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.cors.allowed_origins", []string{"http://localhost:3000"})

	// Bind RapidAPI config to environment variables only (not from config file)
	if err := v.BindEnv("dictionaries.rapidapi.host", "RAPID_API_HOST"); err != nil {
		return nil, fmt.Errorf("failed to bind RAPID_API_HOST environment variable: %w", err)
	}
	if err := v.BindEnv("dictionaries.rapidapi.key", "RAPID_API_KEY"); err != nil {
		return nil, fmt.Errorf("failed to bind RAPID_API_KEY environment variable: %w", err)
	}

	// Bind OpenAI config to environment variables only (not from config file)
	if err := v.BindEnv("openai.api_key", "OPENAI_API_KEY"); err != nil {
		return nil, fmt.Errorf("failed to bind OPENAI_API_KEY environment variable: %w", err)
	}
	if err := v.BindEnv("openai.model", "OPENAI_MODEL"); err != nil {
		return nil, fmt.Errorf("failed to bind OPENAI_MODEL environment variable: %w", err)
	}

	// Bind database password to environment variable
	if err := v.BindEnv("database.password", "DB_PASSWORD"); err != nil {
		return nil, fmt.Errorf("failed to bind DB_PASSWORD environment variable: %w", err)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("configuration file found but could not be read: %w. Please check the file format and permissions", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration format: %w", err)
	}

	if err := loader.validator.Struct(cfg); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		var errorMsgs []string
		for _, e := range validationErrors {
			errorMsgs = append(errorMsgs, e.Translate(loader.translator))
		}
		return nil, fmt.Errorf("invalid configuration: %s", strings.Join(errorMsgs, ", "))
	}

	return &cfg, nil
}
