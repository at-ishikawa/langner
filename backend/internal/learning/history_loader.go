package learning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// HistoryStore returns the LearningHistory shape handlers and quiz code
// expect, but from the database instead of the YAML files in
// learning_notes/. The runtime swap from YAML to DB happens by
// replacing notebook.NewLearningHistories(dir) calls with a HistoryStore
// instance — the result map shape is identical so downstream consumers
// (filters, validators, handlers) don't have to change.
type HistoryStore interface {
	// LoadAll returns every notebook's histories keyed by notebook ID.
	// Mirrors notebook.NewLearningHistories return shape so the swap is
	// drop-in.
	LoadAll(ctx context.Context) (map[string][]notebook.LearningHistory, error)
}

// DBHistoryStore composes the DB repositories needed to reconstruct the
// learning_notes/*.yml view from rows.
type DBHistoryStore struct {
	noteRepo     notebook.NoteRepository
	learningRepo LearningRepository
	originRepo   notebook.EtymologyOriginRepository
	skipFlagRepo notebook.SkipFlagRepository
	// grammarRepo is optional (nil when grammar data isn't present). When
	// set, grammar corrections are reconstructed from grammar_corrections +
	// correction_id logs into flat `type: grammar` histories — the DB is
	// their store now, exactly like notes for vocab and etymology_origins
	// for origins.
	grammarRepo notebook.GrammarCorrectionRepository
}

// NewDBHistoryStore constructs the store. originRepo and grammarRepo are
// optional — pass nil when that data isn't present; the corresponding
// histories are just omitted.
func NewDBHistoryStore(noteRepo notebook.NoteRepository, learningRepo LearningRepository, originRepo notebook.EtymologyOriginRepository, skipFlagRepo notebook.SkipFlagRepository, grammarRepo notebook.GrammarCorrectionRepository) *DBHistoryStore {
	return &DBHistoryStore{
		noteRepo:     noteRepo,
		learningRepo: learningRepo,
		originRepo:   originRepo,
		skipFlagRepo: skipFlagRepo,
		grammarRepo:  grammarRepo,
	}
}

// LoadAll rebuilds the per-notebook LearningHistory map from DB rows.
// Story notebooks land in the .Scenes shape (one LearningScene per
// notebook_notes.subgroup); flashcard notebooks land in the flat
// .Expressions shape with Metadata.Type = "flashcard".
func (s *DBHistoryStore) LoadAll(ctx context.Context) (map[string][]notebook.LearningHistory, error) {
	notes, err := s.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}

	logs, err := s.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load learning logs: %w", err)
	}

	// Bucket logs by their target. Vocab logs key on note_id; etymology
	// logs key on origin_id. ImportLearningLogs (legacy path) routed every
	// expression — vocab AND origin — through note_id with a synthetic
	// note when no notebook_notes link existed; mergeOriginHistories below
	// re-attaches those orphan etymology logs to the matching origin so
	// the reconstructed shape matches what the YAML reader produced.
	logsByNote := make(map[int64][]LearningLog, len(logs))
	logsByOrigin := make(map[int64][]LearningLog)
	logsByCorrection := make(map[int64][]LearningLog)
	for _, l := range logs {
		if l.NoteID != 0 {
			logsByNote[l.NoteID] = append(logsByNote[l.NoteID], l)
			continue
		}
		if l.OriginID != 0 {
			logsByOrigin[l.OriginID] = append(logsByOrigin[l.OriginID], l)
			continue
		}
		if l.CorrectionID != 0 {
			logsByCorrection[l.CorrectionID] = append(logsByCorrection[l.CorrectionID], l)
		}
	}

	// orphanNoteLogsByName lets mergeOriginHistories find logs the
	// legacy importer stashed on synthetic notes (Usage = origin name,
	// no notebook_notes link). Keyed by lower(Usage).
	orphanNoteLogsByName := make(map[string][]LearningLog)
	for _, n := range notes {
		if len(n.NotebookNotes) > 0 {
			continue
		}
		ls := logsByNote[n.ID]
		if len(ls) == 0 {
			continue
		}
		orphanNoteLogsByName[strings.ToLower(strings.TrimSpace(n.Usage))] = append(
			orphanNoteLogsByName[strings.ToLower(strings.TrimSpace(n.Usage))], ls...,
		)
	}

	noteIDs := make([]int64, 0, len(notes))
	for _, n := range notes {
		noteIDs = append(noteIDs, n.ID)
	}
	noteSkipFlags, err := s.skipFlagRepo.FindNoteFlags(ctx, noteIDs)
	if err != nil {
		return nil, fmt.Errorf("load note skip flags: %w", err)
	}
	skipFlagsByNote := make(map[int64]notebook.SkippedAtMap, len(noteSkipFlags))
	for _, f := range noteSkipFlags {
		m := skipFlagsByNote[f.NoteID]
		if m == nil {
			m = make(notebook.SkippedAtMap)
		}
		m[f.QuizType] = f.SkippedAt.UTC().Format(time.RFC3339)
		skipFlagsByNote[f.NoteID] = m
	}

	histories := make(map[string][]notebook.LearningHistory)
	buildVocabHistories(notes, logsByNote, skipFlagsByNote, histories)

	if s.originRepo != nil {
		origins, oerr := s.originRepo.FindAll(ctx)
		if oerr != nil {
			return nil, fmt.Errorf("load etymology origins: %w", oerr)
		}
		originIDs := make([]int64, 0, len(origins))
		for _, o := range origins {
			originIDs = append(originIDs, o.ID)
		}
		originSkipFlags, oferr := s.skipFlagRepo.FindOriginFlags(ctx, originIDs)
		if oferr != nil {
			return nil, fmt.Errorf("load origin skip flags: %w", oferr)
		}
		skipFlagsByOrigin := make(map[int64]notebook.SkippedAtMap, len(originSkipFlags))
		for _, f := range originSkipFlags {
			m := skipFlagsByOrigin[f.OriginID]
			if m == nil {
				m = make(notebook.SkippedAtMap)
			}
			m[f.QuizType] = f.SkippedAt.UTC().Format(time.RFC3339)
			skipFlagsByOrigin[f.OriginID] = m
		}
		mergeOriginHistories(origins, logsByOrigin, orphanNoteLogsByName, skipFlagsByOrigin, histories)
	}

	if s.grammarRepo != nil {
		corrections, gerr := s.grammarRepo.FindAll(ctx)
		if gerr != nil {
			return nil, fmt.Errorf("load grammar corrections: %w", gerr)
		}
		buildGrammarHistories(corrections, logsByCorrection, histories)
	}

	return histories, nil
}

// buildGrammarHistories reconstructs one flat `type: grammar` LearningHistory
// per grammar/journal notebook from grammar_corrections + their correction_id
// logs — the DB-only-state replacement for the old YAML merge. Each correction
// becomes one expression keyed by its sense_id (the correction's stable id,
// which every consumer — the grammar quiz's due filter, grammar Relearn,
// Analytics — reads). A grammar log lives in the LearnedLogs slot carrying
// quiz_type=grammar, matching GetLogsForQuizType(grammar).
func buildGrammarHistories(
	corrections []notebook.GrammarCorrectionRecord,
	logsByCorrection map[int64][]LearningLog,
	out map[string][]notebook.LearningHistory,
) {
	// Group corrections by notebook so each notebook gets ONE grammar history
	// carrying all its corrections; preserve first-seen order per notebook and
	// sort notebooks for a deterministic map.
	type nbGrammar struct {
		exprs []notebook.LearningHistoryExpression
	}
	byNotebook := make(map[string]*nbGrammar)
	var nbOrder []string
	for _, c := range corrections {
		exp := buildGrammarExpression(c.SenseID, logsByCorrection[c.ID])
		g, ok := byNotebook[c.NotebookID]
		if !ok {
			g = &nbGrammar{}
			byNotebook[c.NotebookID] = g
			nbOrder = append(nbOrder, c.NotebookID)
		}
		g.exprs = append(g.exprs, exp)
	}
	sort.Strings(nbOrder)
	for _, nbID := range nbOrder {
		out[nbID] = append(out[nbID], notebook.LearningHistory{
			Metadata: notebook.LearningHistoryMetadata{
				NotebookID: nbID,
				Type:       "grammar",
			},
			Expressions: byNotebook[nbID].exprs,
		})
	}
}

// buildGrammarExpression builds one grammar correction's expression. Its logs
// are ordered NEWEST-FIRST by LearnedAt so GetLatestStatus / NeedsForwardReview
// (which read LearnedLogs[0] as the latest attempt) decide a correction's
// current status correctly — a runtime miss written with the highest DB id
// must not be shadowed by an older log (the DB-log-ordering rule applied
// everywhere in DB-only-state). id and expression are both the sense_id, the
// correction's canonical key.
func buildGrammarExpression(senseID string, logs []LearningLog) notebook.LearningHistoryExpression {
	sorted := append([]LearningLog{}, logs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].LearnedAt.After(sorted[j].LearnedAt)
	})
	learnedLogs := make([]notebook.LearningRecord, 0, len(sorted))
	for _, l := range sorted {
		learnedLogs = append(learnedLogs, notebook.LearningRecord{
			Status:         notebook.LearnedStatus(l.Status),
			LearnedAt:      notebook.NewDate(l.LearnedAt),
			Quality:        l.Quality,
			ResponseTimeMs: int64(l.ResponseTimeMs),
			QuizType:       l.QuizType,
			IntervalDays:   l.IntervalDays,
		})
	}
	return notebook.LearningHistoryExpression{
		ID:          senseID,
		Expression:  senseID,
		LearnedLogs: learnedLogs,
	}
}

// buildVocabHistories walks notes + their notebook_notes links and
// materialises a LearningHistory per (notebook, event) — scenes hang off
// notebook_notes.subgroup. Flashcard notebooks collapse all expressions
// into the flat .Expressions slice with Metadata.Type = "flashcard".
func buildVocabHistories(
	notes []notebook.NoteRecord,
	logsByNote map[int64][]LearningLog,
	skipFlagsByNote map[int64]notebook.SkippedAtMap,
	out map[string][]notebook.LearningHistory,
) {
	type historyKey struct {
		notebookID string
		title      string
	}
	type sceneKey struct {
		notebookID string
		title      string
		scene      string
	}

	// Preserve the order notes were inserted under each (notebookID,
	// title, scene) bucket so the output is deterministic.
	titleOrder := make(map[string][]historyKey)
	sceneOrder := make(map[historyKey][]sceneKey)
	seenTitles := make(map[historyKey]bool)
	seenScenes := make(map[sceneKey]bool)

	titleType := make(map[historyKey]string)
	exprByScene := make(map[sceneKey][]notebook.LearningHistoryExpression)
	flashcardExprByTitle := make(map[historyKey][]notebook.LearningHistoryExpression)

	for _, n := range notes {
		expr := newExpressionFromNote(n, logsByNote[n.ID], skipFlagsByNote[n.ID])
		for _, nn := range n.NotebookNotes {
			hk := historyKey{nn.NotebookID, nn.Group}
			if !seenTitles[hk] {
				seenTitles[hk] = true
				titleOrder[nn.NotebookID] = append(titleOrder[nn.NotebookID], hk)
			}
			titleType[hk] = nn.NotebookType
			if nn.NotebookType == "flashcard" {
				flashcardExprByTitle[hk] = append(flashcardExprByTitle[hk], expr)
				continue
			}
			sk := sceneKey{nn.NotebookID, nn.Group, nn.Subgroup}
			if !seenScenes[sk] {
				seenScenes[sk] = true
				sceneOrder[hk] = append(sceneOrder[hk], sk)
			}
			exprByScene[sk] = append(exprByScene[sk], expr)
		}
	}

	notebookIDs := make([]string, 0, len(titleOrder))
	for id := range titleOrder {
		notebookIDs = append(notebookIDs, id)
	}
	sort.Strings(notebookIDs)

	for _, nbID := range notebookIDs {
		for _, hk := range titleOrder[nbID] {
			nbType := titleType[hk]
			if nbType == "flashcard" {
				out[nbID] = append(out[nbID], notebook.LearningHistory{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: nbID,
						Title:      hk.title,
						Type:       "flashcard",
					},
					Expressions: flashcardExprByTitle[hk],
				})
				continue
			}
			var scenes []notebook.LearningScene
			for _, sk := range sceneOrder[hk] {
				scenes = append(scenes, notebook.LearningScene{
					Metadata:    notebook.LearningSceneMetadata{Title: sk.scene},
					Expressions: exprByScene[sk],
				})
			}
			out[nbID] = append(out[nbID], notebook.LearningHistory{
				Metadata: notebook.LearningHistoryMetadata{
					NotebookID: nbID,
					Title:      hk.title,
				},
				Scenes: scenes,
			})
		}
	}
}

// mergeOriginHistories adds origin-typed LearningHistoryExpression rows
// into the existing per-notebook histories. Origins land in the scene
// matching their (notebook_id, session_title) tuple — same convention
// the YAML reader used.
func mergeOriginHistories(
	origins []notebook.EtymologyOriginRecord,
	logsByOrigin map[int64][]LearningLog,
	orphanLogsByName map[string][]LearningLog,
	skipFlagsByOrigin map[int64]notebook.SkippedAtMap,
	out map[string][]notebook.LearningHistory,
) {
	for _, o := range origins {
		// Combine logs the importer wrote with origin_id (forward-only
		// path) and those it stashed on a synthetic note keyed by name
		// (legacy path). Same origin may have both kinds during the
		// transition window.
		logs := append([]LearningLog{}, logsByOrigin[o.ID]...)
		if orphan := orphanLogsByName[strings.ToLower(strings.TrimSpace(o.Origin))]; len(orphan) > 0 {
			logs = append(logs, orphan...)
		}
		expr := newExpressionFromOrigin(o, logs, skipFlagsByOrigin[o.ID])
		histories := out[o.NotebookID]
		matched := false
		for i := range histories {
			if !sessionMatches(histories[i], o.SessionTitle) {
				continue
			}
			matched = true
			for j := range histories[i].Scenes {
				if histories[i].Scenes[j].Metadata.Title == o.SessionTitle {
					histories[i].Scenes[j].Expressions = append(histories[i].Scenes[j].Expressions, expr)
					goto next
				}
			}
			histories[i].Scenes = append(histories[i].Scenes, notebook.LearningScene{
				Metadata:    notebook.LearningSceneMetadata{Title: o.SessionTitle},
				Expressions: []notebook.LearningHistoryExpression{expr},
			})
		next:
		}
		if !matched {
			histories = append(histories, notebook.LearningHistory{
				Metadata: notebook.LearningHistoryMetadata{NotebookID: o.NotebookID, Title: o.SessionTitle},
				Scenes: []notebook.LearningScene{{
					Metadata:    notebook.LearningSceneMetadata{Title: o.SessionTitle},
					Expressions: []notebook.LearningHistoryExpression{expr},
				}},
			})
		}
		out[o.NotebookID] = histories
	}
}

func sessionMatches(h notebook.LearningHistory, sessionTitle string) bool {
	if strings.EqualFold(h.Metadata.Title, sessionTitle) {
		return true
	}
	for _, sc := range h.Scenes {
		if strings.EqualFold(sc.Metadata.Title, sessionTitle) {
			return true
		}
	}
	return false
}

func newExpressionFromNote(n notebook.NoteRecord, logs []LearningLog, skipFlags notebook.SkippedAtMap) notebook.LearningHistoryExpression {
	entry := n.Entry
	if entry == "" {
		entry = n.Usage
	}
	exp := buildExpressionFromDBOrder(entry, logs)
	exp.Type = notebook.LearningExpressionTypeVocabulary
	exp.SkippedAt = skipFlags
	return exp
}

func newExpressionFromOrigin(o notebook.EtymologyOriginRecord, logs []LearningLog, skipFlags notebook.SkippedAtMap) notebook.LearningHistoryExpression {
	exp := buildExpressionFromDBOrder(o.Origin, logs)
	exp.Type = notebook.LearningExpressionTypeOrigin
	exp.SkippedAt = skipFlags
	return exp
}

// buildExpressionFromDBOrder is the DB-side companion to
// learning.buildExpression. Every consumer reads the "latest" attempt as slot
// [0] (GetLatestStatus / NeedsForwardReview / NeedsReverseReview), and the YAML
// reader surfaces newest-first. Each per-quiz-type bucket is therefore sorted
// NEWEST-FIRST by LearnedAt.
//
// Why an explicit sort (not id order): imported logs land in YAML order so
// ORDER BY id ASC happens to be newest-first, but a RUNTIME attempt is INSERTed
// after import and gets the HIGHEST id, so id order would push it to the END and
// [0] would keep surfacing the stale imported log — a correct answer would then
// never advance the word's due date (it stayed due forever in DB mode). Sorting
// by LearnedAt makes the just-written attempt the latest regardless of insert
// order, exactly like buildGrammarExpression already does.
func buildExpressionFromDBOrder(expression string, logs []LearningLog) notebook.LearningHistoryExpression {
	var learnedLogs, reverseLogs, originLogs []notebook.LearningRecord
	convert := func(l LearningLog) notebook.LearningRecord {
		return notebook.LearningRecord{
			Status:         notebook.LearnedStatus(l.Status),
			LearnedAt:      notebook.NewDate(l.LearnedAt),
			Quality:        l.Quality,
			ResponseTimeMs: int64(l.ResponseTimeMs),
			QuizType:       l.QuizType,
			IntervalDays:   l.IntervalDays,
		}
	}
	// Route each log to the SAME slot GetLogsForQuizType /
	// SetLogsForQuizType use so the DB-reconstructed expression is
	// symmetric with the writer (learning-history invariant L2):
	// reverse → ReverseLogs, etymology_origin → EtymologyOriginLogs,
	// everything else (notebook, freeform, grammar, legacy empty) →
	// LearnedLogs. Freeform records stay in LearnedLogs carrying their
	// own QuizType, exactly as the YAML reader surfaced them.
	for _, l := range logs {
		rec := convert(l)
		switch notebook.QuizType(l.QuizType) {
		case notebook.QuizTypeReverse:
			reverseLogs = append(reverseLogs, rec)
		case notebook.QuizTypeEtymologyOrigin:
			originLogs = append(originLogs, rec)
		default:
			learnedLogs = append(learnedLogs, rec)
		}
	}
	sortRecordsNewestFirst(learnedLogs)
	sortRecordsNewestFirst(reverseLogs)
	sortRecordsNewestFirst(originLogs)
	return notebook.LearningHistoryExpression{
		Expression:          expression,
		LearnedLogs:         learnedLogs,
		ReverseLogs:         reverseLogs,
		EtymologyOriginLogs: originLogs,
	}
}

// sortRecordsNewestFirst orders learning records by LearnedAt descending
// (newest first) with a stable sort, so records sharing a timestamp keep their
// DB id order. Every "latest attempt is [0]" consumer depends on this.
func sortRecordsNewestFirst(records []notebook.LearningRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].LearnedAt.After(records[j].LearnedAt.Time)
	})
}
