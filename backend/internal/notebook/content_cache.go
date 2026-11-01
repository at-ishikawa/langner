package notebook

import (
	"context"
	"sync"
)

// CachedContent keeps the notebooks it has loaded in memory and serves them
// again while their version (from the latest catalog read) is unchanged, so a
// warm server reads only the catalog per request and reloads a notebook only
// after it is pushed or re-imported. Nothing is loaded until a request reads it.
type CachedContent struct {
	inner ContentLoader

	mu       sync.Mutex
	versions map[string]string // notebook id -> version from the latest catalog
	cached   map[string]cachedNotebook
}

type cachedNotebook struct {
	version string
	content NotebookContent
}

// NewCachedContent wraps a loader with the cache.
func NewCachedContent(inner ContentLoader) *CachedContent {
	return &CachedContent{inner: inner, versions: map[string]string{}, cached: map[string]cachedNotebook{}}
}

// Catalog implements ContentLoader and records each notebook's version.
func (c *CachedContent) Catalog(ctx context.Context) ([]CatalogEntry, error) {
	catalog, err := c.inner.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(catalog))
	for _, e := range catalog {
		versions[e.NotebookID] = e.Version
	}
	c.mu.Lock()
	c.versions = versions
	for id := range c.cached {
		if _, ok := versions[id]; !ok {
			delete(c.cached, id) // deleted notebook
		}
	}
	c.mu.Unlock()
	return catalog, nil
}

// Load implements ContentLoader: cached notebooks at the current version are
// returned from memory; the rest are loaded in one call.
func (c *CachedContent) Load(ctx context.Context, ids []string) ([]NotebookContent, error) {
	var out []NotebookContent
	var missing []string
	versions := make(map[string]string)
	c.mu.Lock()
	for _, id := range ids {
		versions[id] = c.versions[id]
		if nb, ok := c.cached[id]; ok && nb.version == c.versions[id] {
			out = append(out, nb.content)
		} else {
			missing = append(missing, id)
		}
	}
	c.mu.Unlock()
	if len(missing) == 0 {
		return out, nil
	}
	loaded, err := c.inner.Load(ctx, missing)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	for _, nc := range loaded {
		// The version seen before loading: if the notebook changed meanwhile,
		// the next catalog read no longer matches and it is loaded again.
		c.cached[nc.ID] = cachedNotebook{version: versions[nc.ID], content: nc}
	}
	c.mu.Unlock()
	return append(out, loaded...), nil
}
