package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Notebooks    NotebooksConfig    `mapstructure:"notebooks"`
	Dictionaries DictionariesConfig `mapstructure:"dictionaries"`
	Templates    TemplatesConfig    `mapstructure:"templates"`
	Outputs      OutputsConfig      `mapstructure:"outputs"`
	OpenAI       OpenAIConfig       `mapstructure:"openai"`
}

type NotebooksConfig struct {
	StoriesDirectory       string `mapstructure:"stories_directory"`
	LearningNotesDirectory string `mapstructure:"learning_notes_directory"`
}

type TemplatesConfig struct {
	MarkdownDirectory string `mapstructure:"markdown_directory"`
}

type OutputsConfig struct {
	StoryDirectory string `mapstructure:"story_directory"`
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

func Load(configFile string) (*Config, error) {
	v := viper.New()

	v.SetConfigType("yaml")

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/langner")
	}

	v.SetDefault("notebooks.stories_directory", filepath.Join("notebooks", "stories"))
	v.SetDefault("notebooks.learning_notes_directory", filepath.Join("notebooks", "learning_notes"))
	v.SetDefault("dictionaries.rapidapi.cache_directory", filepath.Join("dictionaries", "rapidapi"))
	v.SetDefault("templates.markdown_directory", filepath.Join("assets", "templates"))
	v.SetDefault("outputs.story_directory", filepath.Join("outputs", "story"))
	v.SetDefault("openai.model", "gpt-4o-mini")

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

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("configuration file found but could not be read: %w. Please check the file format and permissions", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration format: %w", err)
	}

	return &cfg, nil
}
