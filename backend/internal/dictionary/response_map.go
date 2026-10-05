package dictionary

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

// LoadResponseMap returns the stored dictionary keyed by word, in the SAME shape
// the filesystem cache produces (rapidapi.FromResponsesToMap) so the notebook
// reader resolves a word's meaning identically whether the dictionary comes from
// the on-disk cache or Postgres. In a served environment (dev / e2e / prod) the
// cache directory is absent, so the story reader's Note.SetDetails can only fill
// a dictionary-backed word's meaning from here — without it the whole notebook
// listing fails for any word whose meaning lives in the dictionary.
//
// A row whose stored JSON can't be unmarshalled is skipped (logged by the
// caller via the returned count gap) rather than failing the whole load. The map
// key is the entry's word column, which is what the notebook Definition looks up.
func (r *DBDictionaryRepository) LoadResponseMap(ctx context.Context) (map[string]rapidapi.Response, error) {
	entries, err := r.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load dictionary entries: %w", err)
	}
	m := make(map[string]rapidapi.Response, len(entries))
	for _, e := range entries {
		var resp rapidapi.Response
		if err := json.Unmarshal(e.Response, &resp); err != nil {
			continue // skip a malformed stored response rather than fail the load
		}
		if resp.Word == "" {
			resp.Word = e.Word
		}
		m[e.Word] = resp
	}
	return m, nil
}
