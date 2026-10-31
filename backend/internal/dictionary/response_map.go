package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/at-ishikawa/langner/internal/database"
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
	responses, err := r.loadResponses(ctx)
	if err != nil {
		return nil, fmt.Errorf("load dictionary entries: %w", err)
	}
	m := make(map[string]rapidapi.Response, len(responses))
	for word, raw := range responses {
		var resp rapidapi.Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue // skip a malformed stored response rather than fail the load
		}
		if resp.Word == "" {
			resp.Word = word
		}
		m[word] = resp
	}
	return m, nil
}

// loadResponses returns every stored response keyed by word. It reads the gzip
// copy (migration 037) and decompresses it here, so far fewer bytes cross the
// wire; a row without a copy yet is read uncompressed and backfilled,
// best-effort.
func (r *DBDictionaryRepository) loadResponses(ctx context.Context) (map[string][]byte, error) {
	if !responseGzColumn.Exists(ctx, r.db) {
		entries, err := r.FindAll(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[string][]byte, len(entries))
		for _, e := range entries {
			out[e.Word] = e.Response
		}
		return out, nil
	}

	var rows []struct {
		Word       string `db:"word"`
		Response   []byte `db:"response"`
		ResponseGz []byte `db:"response_gz"`
	}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT word, response_gz, CASE WHEN response_gz IS NULL THEN response END AS response
		 FROM dictionary_entries ORDER BY word`); err != nil {
		return nil, fmt.Errorf("load dictionary responses: %w", err)
	}
	out := make(map[string][]byte, len(rows))
	for _, row := range rows {
		if row.ResponseGz != nil {
			raw, err := database.Gunzip(row.ResponseGz)
			if err != nil {
				continue // skip a corrupt copy rather than fail the load
			}
			out[row.Word] = raw
			continue
		}
		out[row.Word] = row.Response
		if _, err := r.db.ExecContext(ctx,
			`UPDATE dictionary_entries SET response_gz = $1 WHERE word = $2 AND response_gz IS NULL`,
			database.Gzip(row.Response), row.Word); err != nil {
			slog.Warn("backfill dictionary_entries.response_gz failed", "word", row.Word, "error", err)
		}
	}
	return out, nil
}
