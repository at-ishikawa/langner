package notebook

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"bank":              "bank",
		"Break the Ice":     "break-the-ice",
		"lose one's temper": "lose-one-s-temper",
		"  spaced  out  ":   "spaced-out",
		"!!!":               "",
		"CO2":               "co2",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAssignSourceIDs_DedupsGlobally(t *testing.T) {
	entries := []SourceEntry{
		{Expression: "bank"},               // -> bank
		{Expression: "bank"},               // -> bank-2 (collision)
		{Expression: "keep", ID: "keep-x"}, // keeps existing id
		{Expression: "bank"},               // -> bank-3
	}
	used := make(map[string]bool)
	got := AssignSourceIDs(entries, used)
	want := []string{"bank", "bank-2", "keep-x", "bank-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// A second notebook sharing the same used map must not reuse "bank".
	more := AssignSourceIDs([]SourceEntry{{Expression: "bank"}}, used)
	if more[0] != "bank-4" {
		t.Errorf("cross-notebook dedup: got %q, want %q", more[0], "bank-4")
	}
}

// TestAddIDsToSourceYAML_AddOnly is the critical round-trip guarantee: writing
// ids into hand-authored source YAML changes ONLY by adding `id:` lines. Every
// other line must be byte-identical to the original.
func TestAddIDsToSourceYAML_AddOnly(t *testing.T) {
	src := `notes:
  - expression: bank
    meaning: the land alongside a river
    examples:
      - we sat on the bank
  - expression: bank
    meaning: a financial institution
  - id: keep-existing
    expression: keep
    meaning: to retain
`
	used := make(map[string]bool)
	out, added, err := AddIDsToSourceYAML([]byte(src), used)
	if err != nil {
		t.Fatalf("AddIDsToSourceYAML: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2 (the two id-less entries)", added)
	}

	// The only permitted change is the insertion of an `id:` line (rendered
	// inline with the sequence dash) as each id-less entry's first key. Assert
	// the output is byte-for-byte the source with exactly those insertions —
	// no reformatting of any other line.
	want := "notes:\n" +
		"  - id: bank\n" +
		"    expression: bank\n" +
		"    meaning: the land alongside a river\n" +
		"    examples:\n" +
		"      - we sat on the bank\n" +
		"  - id: bank-2\n" +
		"    expression: bank\n" +
		"    meaning: a financial institution\n" +
		"  - id: keep-existing\n" +
		"    expression: keep\n" +
		"    meaning: to retain\n"
	if string(out) != want {
		t.Errorf("round-trip changed more than the id lines.\n--- got ---\n%s\n--- want ---\n%s", string(out), want)
	}
}

func TestAddIDsToSourceYAML_Idempotent(t *testing.T) {
	src := `notes:
  - expression: bank
    meaning: money place
`
	used := make(map[string]bool)
	out1, added1, err := AddIDsToSourceYAML([]byte(src), used)
	if err != nil || added1 != 1 {
		t.Fatalf("first pass: added=%d err=%v", added1, err)
	}
	// Second pass over the already-stamped output adds nothing and returns the
	// bytes unchanged.
	out2, added2, err := AddIDsToSourceYAML(out1, used)
	if err != nil {
		t.Fatalf("second pass err: %v", err)
	}
	if added2 != 0 {
		t.Errorf("second pass added = %d, want 0", added2)
	}
	if string(out2) != string(out1) {
		t.Errorf("second pass mutated bytes:\n%s", string(out2))
	}
}

func TestRekeyLearningHistories(t *testing.T) {
	histories := []LearningHistory{
		{
			Expressions: []LearningHistoryExpression{
				{Expression: "bank"},          // ambiguous -> left id-less
				{Expression: "keep"},          // single -> stamped
				{Expression: "already", ID: "already-1"}, // untouched
			},
			Scenes: []LearningScene{
				{Expressions: []LearningHistoryExpression{
					{Expression: "river"}, // single -> stamped
					{Expression: "gone"},  // no candidate -> left id-less
				}},
			},
		},
	}
	idByExpr := map[string][]string{
		"bank":  {"bank", "bank-2"}, // ambiguous
		"keep":  {"keep"},
		"river": {"river"},
	}
	n := RekeyLearningHistories(histories, idByExpr)
	if n != 2 {
		t.Fatalf("stamped = %d, want 2", n)
	}
	if histories[0].Expressions[0].ID != "" {
		t.Errorf("ambiguous 'bank' should stay id-less, got %q", histories[0].Expressions[0].ID)
	}
	if histories[0].Expressions[1].ID != "keep" {
		t.Errorf("'keep' id = %q, want keep", histories[0].Expressions[1].ID)
	}
	if histories[0].Expressions[2].ID != "already-1" {
		t.Errorf("'already' id changed to %q", histories[0].Expressions[2].ID)
	}
	if histories[0].Scenes[0].Expressions[0].ID != "river" {
		t.Errorf("'river' id = %q, want river", histories[0].Scenes[0].Expressions[0].ID)
	}
	if histories[0].Scenes[0].Expressions[1].ID != "" {
		t.Errorf("'gone' should stay id-less, got %q", histories[0].Scenes[0].Expressions[1].ID)
	}
}
