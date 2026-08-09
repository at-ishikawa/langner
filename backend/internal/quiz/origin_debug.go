package quiz

// TEMPORARY diagnostic instrumentation for origin-family grouping in Relearn.
// Everything here is gated behind the LANGNER_ORIGIN_DEBUG=1 environment
// variable and only writes to stderr — behaviour is unchanged when the variable
// is unset. This whole file is a debugging aid and is intended to be removed
// once the "origin-bearing words are not grouping by origin" issue is diagnosed.
//
// Enable with:  LANGNER_ORIGIN_DEBUG=1
// Optionally target specific words (comma-separated, case-insensitive):
//   LANGNER_ORIGIN_DEBUG_WORDS=someword,anotherword
// Grep the backend stderr for the single tag: [origin-debug]

import (
	"fmt"
	"os"
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// originDebugEnabled reports whether the diagnostic instrumentation is on.
func originDebugEnabled() bool {
	return os.Getenv("LANGNER_ORIGIN_DEBUG") == "1"
}

// originDebugWords parses LANGNER_ORIGIN_DEBUG_WORDS into a lowercased set. An
// empty/unset value yields an empty set (meaning: no explicit watchlist).
func originDebugWords() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("LANGNER_ORIGIN_DEBUG_WORDS"))
	if raw == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, w := range strings.Split(raw, ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			set[w] = struct{}{}
		}
	}
	return set
}

// originDebugLogf writes one distinctive, greppable line to stderr.
func originDebugLogf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[origin-debug] "+format+"\n", args...)
}

// originDebugLogWord logs a single finalized vocabulary word's raw (unresolved)
// origin refs versus its resolved origin parts, so we can tell apart:
//
//	(b) raw refs present but resolution dropped them (originMap key miss), vs
//	(c) raw refs absent entirely (loader not reading origin_parts).
//
// To avoid flooding, a word is logged only when it HAS raw origin refs, or when
// its expression is in the LANGNER_ORIGIN_DEBUG_WORDS watchlist.
func originDebugLogWord(expr, id, notebookName string, raw []notebook.OriginPartRef, resolved []WordOriginPart) {
	if !originDebugEnabled() {
		return
	}
	watch := originDebugWords()
	_, watched := watch[strings.ToLower(strings.TrimSpace(expr))]
	if len(raw) == 0 && !watched {
		return
	}
	originDebugLogf("word expr=%q id=%q notebook=%q rawOriginRefs=%v resolvedOriginParts=%v",
		expr, id, notebookName, raw, resolved)
}
