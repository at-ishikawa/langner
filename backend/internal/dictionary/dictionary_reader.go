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
	// dictRepo, when set (NewDBReader), makes Lookup DB-backed: it reads/writes
	// dictionary_entries instead of the filesystem cache, so the serving backend
	// never depends on an on-disk rapidapi cache (absent on a serverless deploy)
	// and a freshly looked-up word is persisted for every later request.
	dictRepo *DBDictionaryRepository
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

// NewDBReader builds a reader that caches lookups in Postgres (dictionary_entries)
// instead of the filesystem, so the serving backend has no on-disk dictionary
// cache dependency.
func NewDBReader(repo *DBDictionaryRepository, config Config) *Reader {
	return &Reader{
		config:   config,
		dictRepo: repo,
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
	if r.dictRepo != nil {
		return r.lookupDB(ctx, expression)
	}
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

// lookupDB resolves a word against dictionary_entries: a hit returns the stored
// response; a miss fetches from the external API and PERSISTS it, so the word is
// served from the DB on every later request. No filesystem cache is touched.
func (r *Reader) lookupDB(ctx context.Context, expression string) (rapidapi.Response, error) {
	var resp rapidapi.Response
	body, ok, err := r.dictRepo.FindResponseByWord(ctx, expression)
	if err != nil {
		return resp, fmt.Errorf("dictRepo.FindResponseByWord > %w", err)
	}
	if !ok {
		body, err = r.lookupAPI(ctx, expression)
		if err != nil {
			return resp, fmt.Errorf("r.lookupAPI > %w", err)
		}
		if err := r.dictRepo.BatchUpsert(ctx, []*DictionaryEntry{
			{Word: expression, SourceType: "rapidapi", Response: body},
		}); err != nil {
			return resp, fmt.Errorf("persist dictionary entry > %w", err)
		}
	}
	if err := json.Unmarshal(body, &resp); err != nil {
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
