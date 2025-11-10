package main

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type API string

func (a *API) Set(val string) error {
	for _, api := range allAPIs {
		if val == string(api) {
			*a = api
			return nil
		}
	}
	return fmt.Errorf("invalid API: %s", val)
}

func (a API) String() string {
	return string(a)
}

func (a *API) Type() string {
	return "API"
}

const (
	// APIFreeDictionaryAPI  API = "free_dictionary"
	APIWordsAPIInRapidAPI API = "words_api"
)

var (
	_       pflag.Value = (*API)(nil)
	allAPIs             = []API{APIWordsAPIInRapidAPI}
)

func newDictionaryCommand() *cobra.Command {
	rootCommand := cobra.Command{
		Use: "dictionary",
	}
	flags := rootCommand.PersistentFlags()

	api := APIWordsAPIInRapidAPI
	flags.Var(&api, "api", fmt.Sprintf("API to use. Possible values are %v", allAPIs))

	rootCommand.AddCommand(&cobra.Command{
		Use:  "lookup",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			word := args[0]

			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("failed to create config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			ctx := cmd.Context()
			var definitions rapidapi.Response

			switch api {
			case APIWordsAPIInRapidAPI:
				fallthrough
			default:
				reader := dictionary.NewReader(cfg.Dictionaries.RapidAPI.CacheDirectory, dictionary.Config{
					RapidAPIHost: cfg.Dictionaries.RapidAPI.Host,
					RapidAPIKey:  cfg.Dictionaries.RapidAPI.Key,
				})
				definitions, err = reader.Lookup(ctx, word)
				if err != nil {
					return fmt.Errorf("dictionary.NewReader.Lookup > %w", err)
				}
				reader.Show(definitions)
			}
			return nil
		},
	})
	return &rootCommand
}
