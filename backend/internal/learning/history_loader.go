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
}

// NewDBHistoryStore constructs the store. originRepo is optional — pass
// nil when etymology data isn't present; origin-typed expressions will
// just be omitted.
func NewDBHistoryStore(noteRepo notebook.NoteRepository, learningRepo LearningRepository, originRepo notebook.EtymologyOriginRepository, skipFlagRepo notebook.SkipFlagRepository) *DBHistoryStore {
	return &DBHistoryStore{
		noteRepo:     noteRepo,
		learningRepo: learningRepo,
		originRepo:   originRepo,
		skipFlagRepo: skipFlagRepo,
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
	for _, l := range logs {
		if l.NoteID != 0 {
			logsByNote[l.NoteID] = append(logsByNote[l.NoteID], l)
			continue
		}
		if l.OriginID != 0 {
			logsByOrigin[l.OriginID] = append(logsByOrigin[l.OriginID], l)
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

	return histories, nil
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
// learning.buildExpression. The YAML reader keeps records in their YAML
// array order ("first" is "newest" — new entries are PREPENDED on write).
// Logs land in the DB during import in YAML order; their primary key is
// monotonically increasing, so ORDER BY id ASC reproduces the YAML array
// order. We therefore split logs into the per-quiz-type buckets without
// re-sorting so callers' GetLatestStatus / GetLatestLogs sees the same
// [0] entry the YAML loader would have surfaced.
func buildExpressionFromDBOrder(expression string, logs []LearningLog) notebook.LearningHistoryExpression {
	var learnedLogs, reverseLogs, breakdownLogs, assemblyLogs []notebook.LearningRecord
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
	for _, l := range logs {
		rec := convert(l)
		switch l.QuizType {
		case string(notebook.QuizTypeReverse):
			reverseLogs = append(reverseLogs, rec)
		case string(notebook.QuizTypeFreeform):
			// Freeform tests both directions: append to LearnedLogs
			// AND ReverseLogs so GetLogsForQuizType returns the entry
			// regardless of which slot the caller queries. Mirrors how
			// SaveResult's updater wrote two records when the YAML
			// path was authoritative.
			learnedLogs = append(learnedLogs, rec)
			reverseLogs = append(reverseLogs, rec)
		case string(notebook.QuizTypeEtymologyStandard):
			breakdownLogs = append(breakdownLogs, rec)
		case string(notebook.QuizTypeEtymologyReverse):
			assemblyLogs = append(assemblyLogs, rec)
		case string(notebook.QuizTypeEtymologyFreeform):
			// Etymology Freeform writes a single DB row with this
			// quiz_type; SetLogsForQuizType / GetLogsForQuizType
			// expect both etymology slots populated. Duplicate the
			// record into both so a follow-up Override or read sees
			// the entry on either lookup side.
			breakdownLogs = append(breakdownLogs, rec)
			assemblyLogs = append(assemblyLogs, rec)
		default:
			learnedLogs = append(learnedLogs, rec)
		}
	}
	return notebook.LearningHistoryExpression{
		Expression:             expression,
		LearnedLogs:            learnedLogs,
		ReverseLogs:            reverseLogs,
		EtymologyBreakdownLogs: breakdownLogs,
		EtymologyAssemblyLogs:  assemblyLogs,
	}
}
