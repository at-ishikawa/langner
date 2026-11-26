package quiz

import (
	"regexp"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
)

// Fixed reasons of the deterministic reverse matcher, written to the learner.
const (
	reverseReasonExact      = "Exact match."
	reverseReasonSameForm   = "Same expression, in a different form."
	reverseReasonMisspelled = "Misspelled — compare your spelling with the answer."
)

// reverseAcceptedForms returns every form a reverse card accepts as its answer:
// the lemma, the alt form, the highlight of each example shown on the card and,
// for a family concept (ConceptHead set), every member. Empty values and
// case-insensitive duplicates are dropped. Non-family concepts list members for
// display only (they may be synonyms or antonyms), so they are not accepted.
func reverseAcceptedForms(card ReverseCard) []string {
	forms := []string{card.Expression, card.AltForm}
	for _, c := range card.Contexts {
		forms = append(forms, c.Highlight)
	}
	if card.ConceptHead != "" {
		forms = append(forms, card.ConceptMembers...)
	}
	var out []string
	seen := make(map[string]bool, len(forms))
	for _, f := range forms {
		f = strings.TrimSpace(f)
		key := strings.ToLower(f)
		if f == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

// matchReverseAnswer settles a reverse answer without the LLM when the outcome
// is certain, the reverse analogue of deterministicGrammarGrade. ok=false means
// "not certain": the answer must be graded by the LLM.
//
// An answer is same_word when it equals any accepted form after normalizing,
// or matches one token by token: equal tokens, a pronoun filling a placeholder
// ("one's", "someone", "oneself"), a regular inflection, or — in a multi-word
// form only — a one-letter typo in a word of 4+ letters. A single-word answer
// that is a near miss of a single-word form is wrong (spelling is part of
// knowing a word); edit distance never accepts a single word.
func matchReverseAnswer(acceptedForms []string, answer string, responseTimeMs int64) (GradeResult, bool) {
	ans := normalizeReverseText(answer)
	if ans == "" {
		return GradeResult{}, false
	}
	ansTokens := strings.Fields(ans)
	var variants []string
	for _, form := range acceptedForms {
		variants = append(variants, optionalPartVariants(normalizeReverseText(form))...)
	}
	for _, v := range variants {
		if v == ans {
			return sameWordGrade(reverseReasonExact, responseTimeMs), true
		}
	}
	for _, v := range variants {
		if tokensMatch(strings.Fields(v), ansTokens) {
			return sameWordGrade(reverseReasonSameForm, responseTimeMs), true
		}
	}
	if len(ansTokens) == 1 {
		for _, v := range variants {
			if !strings.Contains(v, " ") && isMisspelling(v, ans) {
				return GradeResult{
					Correct:        false,
					Reason:         reverseReasonMisspelled,
					Quality:        reverseQuality(false, responseTimeMs),
					Classification: string(inference.ClassificationWrong),
				}, true
			}
		}
	}
	return GradeResult{}, false
}

func sameWordGrade(reason string, responseTimeMs int64) GradeResult {
	return GradeResult{
		Correct:        true,
		Reason:         reason,
		Quality:        reverseQuality(true, responseTimeMs),
		Classification: string(inference.ClassificationSameWord),
	}
}

var curlyApostrophes = strings.NewReplacer("’", "'", "‘", "'", "ʼ", "'")

// normalizeReverseText lowercases, unifies apostrophes, collapses whitespace
// and drops trailing punctuation.
func normalizeReverseText(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(curlyApostrophes.Replace(s))), " ")
	return strings.TrimSpace(strings.TrimRight(s, ".,!?;:\""))
}

var parenthesizedPart = regexp.MustCompile(`\([^)]*\)`)

// optionalPartVariants treats parenthesized parts of a form as optional,
// returning the form with and without them ("(get) cold feet" → "get cold
// feet", "cold feet").
func optionalPartVariants(form string) []string {
	if !strings.Contains(form, "(") {
		return []string{form}
	}
	with := strings.Join(strings.Fields(strings.NewReplacer("(", "", ")", "").Replace(form)), " ")
	without := strings.Join(strings.Fields(parenthesizedPart.ReplaceAllString(form, " ")), " ")
	return []string{with, without}
}

var (
	possessivePlaceholders = map[string]bool{"one's": true, "someone's": true, "somebody's": true}
	possessiveFills        = map[string]bool{"my": true, "your": true, "his": true, "her": true, "its": true, "our": true, "their": true}
	reflexiveFills         = map[string]bool{"myself": true, "yourself": true, "himself": true, "herself": true, "itself": true, "ourselves": true, "yourselves": true, "themselves": true}
	objectPlaceholders     = map[string]bool{"someone": true, "somebody": true}
	objectFills            = map[string]bool{"me": true, "you": true, "him": true, "her": true, "us": true, "them": true}
)

// tokensMatch reports whether the answer tokens match the accepted-form tokens
// one for one (see matchReverseAnswer).
func tokensMatch(form, answer []string) bool {
	if len(form) != len(answer) {
		return false
	}
	multiWord := len(form) > 1
	for i := range form {
		if !tokenMatches(form[i], answer[i], multiWord) {
			return false
		}
	}
	return true
}

func tokenMatches(form, answer string, multiWord bool) bool {
	if form == answer {
		return true
	}
	switch {
	case possessivePlaceholders[form]:
		return possessiveFills[answer] || (len(answer) > 2 && strings.HasSuffix(answer, "'s"))
	case form == "oneself":
		return reflexiveFills[answer]
	case objectPlaceholders[form]:
		return objectFills[answer]
	}
	forms := append([]string{form}, regularInflections(form)...)
	for _, f := range forms {
		if f == answer {
			return true
		}
	}
	if !multiWord || len([]rune(form)) < 4 {
		return false
	}
	for _, f := range forms {
		if damerauLevenshtein(f, answer) <= 1 {
			return true
		}
	}
	return false
}

// regularInflections returns the regular -s / -es / -ed / -d / -ing forms of a
// word, including y→ies/ied, a dropped final e before -ing and a doubled final
// consonant (step → stepped). Irregular forms (pay → paid) are not derived.
func regularInflections(w string) []string {
	n := len(w)
	if n == 0 {
		return nil
	}
	last := w[n-1]
	var out []string
	switch {
	case strings.HasSuffix(w, "s") || strings.HasSuffix(w, "x") || strings.HasSuffix(w, "z") ||
		strings.HasSuffix(w, "ch") || strings.HasSuffix(w, "sh"):
		out = append(out, w+"es")
	case strings.HasSuffix(w, "o"):
		out = append(out, w+"s", w+"es")
	case last == 'y' && n > 1 && !isVowel(w[n-2]):
		out = append(out, w[:n-1]+"ies")
	default:
		out = append(out, w+"s")
	}
	switch {
	case last == 'e':
		out = append(out, w+"d")
	case last == 'y' && n > 1 && !isVowel(w[n-2]):
		out = append(out, w[:n-1]+"ied")
	default:
		out = append(out, w+"ed")
	}
	if last == 'e' && !strings.HasSuffix(w, "ee") {
		out = append(out, w[:n-1]+"ing")
	} else {
		out = append(out, w+"ing")
	}
	// Consonant-vowel-consonant ending: the final consonant may double.
	if n >= 3 && !isVowel(last) && !strings.ContainsRune("wxy", rune(last)) && isVowel(w[n-2]) && !isVowel(w[n-3]) {
		out = append(out, w+string(last)+"ed", w+string(last)+"ing")
	}
	return out
}

func isVowel(c byte) bool {
	return strings.IndexByte("aeiou", c) >= 0
}

// isMisspelling reports whether answer is a near miss of the single word w or
// of one of its regular inflections. The budget is tight (1 edit from 8
// letters, 2 from 12) so a different short real word (affect / effect) is
// left to the LLM, and spelling variants (realise / realize, colour / color)
// are never called misspellings.
func isMisspelling(w, answer string) bool {
	if spellingVariantKey(w) == spellingVariantKey(answer) {
		return false
	}
	for _, f := range append([]string{w}, regularInflections(w)...) {
		budget := 0
		switch n := len([]rune(f)); {
		case n >= 12:
			budget = 2
		case n >= 8:
			budget = 1
		}
		if d := damerauLevenshtein(f, answer); d > 0 && d <= budget {
			return true
		}
	}
	return false
}

var spellingVariantFolds = strings.NewReplacer("ise", "ize", "yse", "yze", "our", "or", "tre", "ter", "ence", "ense", "ogue", "og", "ll", "l", "ae", "e", "oe", "e")

// spellingVariantKey folds common British / American spelling differences so
// two variants of one word compare equal.
func spellingVariantKey(w string) string {
	return spellingVariantFolds.Replace(w)
}

// damerauLevenshtein is the optimal-string-alignment edit distance (insert,
// delete, substitute, swap adjacent) between a and b.
func damerauLevenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
