package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/go-resty/resty/v2"
)

type Reader struct {
	config    Config
	fileCache *FileCache
}

type Config struct {
	RapidAPIHost string
	RapidAPIKey  string
}

func NewReader(cacheDirectory string, config Config) *Reader {
	return &Reader{
		config:    config,
		fileCache: NewFileCache(cacheDirectory),
	}
}

func (r *Reader) lookupAPI(ctx context.Context, word string) ([]byte, error) {
	config := r.config

	var response []byte
	client := resty.New()
	res, err := client.R().
		EnableTrace().
		SetContext(ctx).
		SetHeader("x-rapidapi-host", config.RapidAPIHost).
		SetHeader("x-rapidapi-key", config.RapidAPIKey).
		Get(
			fmt.Sprintf("https://%s/words/%s", config.RapidAPIHost, word),
		)
	if err != nil {
		return response, fmt.Errorf("client.R.Get > %w, response %s", err, string(res.Body()))
	}
	if res.StatusCode() != http.StatusOK {
		return response, fmt.Errorf("status code: %d, body: %s", res.StatusCode(), string(res.Body()))
	}
	return res.Body(), nil
}

func (r *Reader) Lookup(ctx context.Context, expression string) (rapidapi.Response, error) {
	var resp rapidapi.Response
	contents, err := r.fileCache.cache(expression, func() ([]byte, error) {
		body, err := r.lookupAPI(ctx, expression)
		if err != nil {
			return nil, fmt.Errorf("r.lookupAPI > %w", err)
		}
		return body, nil
	})
	if err != nil {
		return resp, fmt.Errorf("r.fileCache.cache > %w", err)
	}
	if err := json.Unmarshal(contents, &resp); err != nil {
		return resp, fmt.Errorf("json.Unmarshal > %w", err)
	}
	return resp, nil
}

func (r *Reader) Show(response rapidapi.Response) {
	for i, result := range response.Results {
		synonyms := strings.Join(result.Synonyms, ", ")
		fmt.Printf("%d: /%s/\t%80s\t%s\n", i+1, result.PartOfSpeech, result.Definition, synonyms)
	}
}
