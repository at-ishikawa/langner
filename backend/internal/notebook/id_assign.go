package notebook

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// id_assign.go holds the pure, side-effect-free logic behind the
// `langner migrate assign-ids` command:
//
//   - Slugify / AssignSourceIDs generate the globally-unique readable slug
//     that becomes a source entry's stable id.
//   - AddIDsToSourceYAML performs the add-only yaml.Node round-trip that
//     stamps those ids into hand-authored source notebooks without
//     reformatting the surrounding lines.
//   - RekeyLearningHistories best-effort back-fills ids onto pre-migration
//     learning-history entries.
//
// All functions are deterministic and file-system free so they can be unit
// tested directly; the command in cmd/langner wires them to config + disk.

// SourceEntry is the minimal view of a source vocabulary entry needed to
// assign it an id: its displayed expression and any id it already carries.
type SourceEntry struct {
	Expression string
	ID         string
}

// Slugify lowercases s and collapses every run of non-alphanumeric ASCII
// characters into a single '-', trimming leading/trailing dashes. It is the
// base form of an id before global de-duplication ("break the ice" ->
// "break-the-ice").
func Slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// nextUniqueSlug returns base if it is unused, otherwise base-2, base-3, …
// until it finds a slug not present in used. An empty base falls back to
// "entry" so a punctuation-only expression still yields a valid id.
func nextUniqueSlug(base string, used map[string]bool) string {
	if base == "" {
		base = "entry"
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

// AssignSourceIDs returns the id to write for each entry, parallel to entries.
// Entries that already carry an id keep it; id-less entries receive a
// globally-unique slug of their expression. used is the set of ids already in
// play globally; it is seeded with every pre-existing id and then extended
// with each freshly-assigned slug, so callers can thread one shared map
// across all notebooks to guarantee global uniqueness.
func AssignSourceIDs(entries []SourceEntry, used map[string]bool) []string {
	if used == nil {
		used = make(map[string]bool)
	}
	for _, e := range entries {
		if e.ID != "" {
			used[e.ID] = true
		}
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		if e.ID != "" {
			out[i] = e.ID
			continue
		}
		slug := nextUniqueSlug(Slugify(e.Expression), used)
		used[slug] = true
		out[i] = slug
	}
	return out
}

// AddIDsToSourceYAML parses one source-notebook file's bytes as a yaml.Node
// tree, finds every vocabulary-entry mapping (a mapping node carrying an
// `expression` key) that lacks an `id`, assigns each a globally-unique slug
// (extending used), and inserts the id as the mapping's first key. It returns
// the re-encoded bytes and the number of ids added.
//
// The operation is add-only: only new `id:` keys are inserted; existing keys,
// values, ordering, and structure are preserved by round-tripping through the
// node tree rather than re-marshalling parsed structs. When nothing is added
// the original bytes are returned unchanged so the caller can skip the write.
//
// Cloud-mount safe: the result is a buffer the caller writes with a single
// os.WriteFile, never an in-place streaming rewrite.
func AddIDsToSourceYAML(data []byte, used map[string]bool) ([]byte, int, error) {
	if used == nil {
		used = make(map[string]bool)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, 0, fmt.Errorf("unmarshal source yaml: %w", err)
	}

	entries := collectEntryMappings(&doc)

	// Register ids already present in this file before assigning new ones so
	// an existing id is never handed out again within the same file.
	for _, m := range entries {
		if id := mappingScalarValue(m, "id"); id != "" {
			used[id] = true
		}
	}

	// Plan one insertion per id-less entry, anchored on its `expression` key
	// line. The new `id:` line is spliced into the ORIGINAL text verbatim
	// (never re-encoded), so every other byte — quote styles, blank lines,
	// comments, block scalars — is preserved exactly. yaml.Node is used only
	// to locate entries and their line/column; encoding whole documents with
	// yaml.v3 reformats hand-authored files, which is not acceptable here.
	type insertion struct {
		afterLine int // 1-based source line to insert after
		indent    int // leading spaces for the new line = key column - 1
		id        string
	}
	var plan []insertion
	for _, m := range entries {
		if mappingHasKey(m, "id") {
			continue
		}
		exprKey := mappingKeyNode(m, "expression")
		if exprKey == nil {
			continue
		}
		base := Slugify(mappingScalarValue(m, "expression"))
		if base == "" {
			continue // no sluggable expression — leave id-less
		}
		slug := nextUniqueSlug(base, used)
		used[slug] = true
		plan = append(plan, insertion{afterLine: exprKey.Line, indent: exprKey.Column - 1, id: slug})
	}

	if len(plan) == 0 {
		return data, 0, nil
	}

	// Apply bottom-to-top so earlier line numbers stay valid as lines shift.
	sort.Slice(plan, func(i, j int) bool { return plan[i].afterLine > plan[j].afterLine })
	lines := strings.Split(string(data), "\n")
	for _, ins := range plan {
		if ins.afterLine < 1 || ins.afterLine > len(lines) {
			continue
		}
		newLine := strings.Repeat(" ", ins.indent) + "id: " + ins.id
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:ins.afterLine]...)
		out = append(out, newLine)
		out = append(out, lines[ins.afterLine:]...)
		lines = out
	}
	return []byte(strings.Join(lines, "\n")), len(plan), nil
}

// CollectExistingIDs returns the ids already present on entries in one source
// file. Callers seed the global used-set with every file's existing ids before
// assigning new ones, so a slug is never handed out that some other file
// already uses. Returns an empty slice for files with no entries.
func CollectExistingIDs(data []byte) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal source yaml: %w", err)
	}
	var out []string
	for _, m := range collectEntryMappings(&doc) {
		if id := mappingScalarValue(m, "id"); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

// collectEntryMappings walks the node tree and returns every mapping node
// that has an `expression` key — i.e. every source vocabulary entry,
// regardless of whether it sits under `notes:`, a scene's `definitions:`, or
// a flashcard block.
func collectEntryMappings(n *yaml.Node) []*yaml.Node {
	var out []*yaml.Node
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		if node == nil {
			return
		}
		if node.Kind == yaml.MappingNode && mappingHasKey(node, "expression") {
			out = append(out, node)
		}
		for _, c := range node.Content {
			walk(c)
		}
	}
	walk(n)
	return out
}

// mappingHasKey reports whether a mapping node has the given key.
func mappingHasKey(m *yaml.Node, key string) bool {
	if m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return true
		}
	}
	return false
}

// mappingScalarValue returns the scalar value for key, or "" if absent.
func mappingScalarValue(m *yaml.Node, key string) string {
	if m.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}

// mappingKeyNode returns the scalar key node for key in mapping m (nil if
// absent). Its Line/Column locate where to splice the new id line.
func mappingKeyNode(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

// RekeyLearningHistories best-effort stamps ids onto id-less learning-history
// entries. idByExpr maps a lowercased expression to the source ids that share
// it within the same notebook. An id-less entry whose expression resolves to
// EXACTLY ONE source id is stamped with that id; entries already carrying an
// id, or whose expression is ambiguous (0 or >1 candidate ids), are left
// unchanged so their commingled legacy history splits on the next answer. No
// log is ever moved. histories is mutated in place; the count stamped is
// returned.
func RekeyLearningHistories(histories []LearningHistory, idByExpr map[string][]string) int {
	stamped := 0
	stamp := func(list []LearningHistoryExpression) {
		for i := range list {
			if list[i].ID != "" {
				continue
			}
			ids := idByExpr[strings.ToLower(strings.TrimSpace(list[i].Expression))]
			if len(ids) != 1 {
				continue
			}
			list[i].ID = ids[0]
			stamped++
		}
	}
	for h := range histories {
		stamp(histories[h].Expressions)
		for s := range histories[h].Scenes {
			stamp(histories[h].Scenes[s].Expressions)
		}
	}
	return stamped
}
