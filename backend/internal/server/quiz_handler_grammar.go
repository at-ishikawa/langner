package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// grammarBlankCtx is everything the handler needs to grade, save, and override
// one blank, kept per ephemeral note_id for the current session.
type grammarBlankCtx struct {
	notebookID   string
	notebookName string
	entryID      string
	content      string
	blank        quiz.GrammarBlank
}

// StartGrammarQuiz loads the due journal posts and assigns each blank an
// ephemeral note_id (the same id scheme as the vocabulary quiz) so Override /
// Skip reuse the existing RPCs.
func (h *QuizHandler) StartGrammarQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartGrammarQuizRequest],
) (*connect.Response[apiv1.StartGrammarQuizResponse], error) {
	// Narrow to specific entries (e.g. w16, w17) when notebook_sections is set;
	// otherwise every entry of each notebook is included.
	notebookIDs, entryTitlesByID, err := resolveNotebookSections(req.Msg.GetNotebookIds(), req.Msg.GetNotebookSections())
	if err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	var posts []quiz.GrammarPost
	for _, notebookID := range notebookIDs {
		loaded, err := h.svc.LoadGrammarPosts(userID, notebookID, entryTitlesByID[notebookID])
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load grammar posts for %q: %w", notebookID, err))
		}
		posts = append(posts, loaded...)
	}

	protoPosts := make([]*apiv1.GrammarPostCard, 0, len(posts))
	h.mu.Lock()
	h.grammarStore = make(map[int64]grammarBlankCtx)
	for _, post := range posts {
		protoBlanks := make([]*apiv1.GrammarBlank, 0, len(post.Blanks))
		for _, blank := range post.Blanks {
			noteID := h.nextID
			h.nextID++
			h.grammarStore[noteID] = grammarBlankCtx{
				notebookID:   post.NotebookID,
				notebookName: post.NotebookName,
				entryID:      post.EntryID,
				content:      post.Content,
				blank:        blank,
			}
			protoBlanks = append(protoBlanks, &apiv1.GrammarBlank{
				NoteId:    noteID,
				SenseId:   blank.SenseID,
				Incorrect: blank.Incorrect,
				Category:  blank.Category,
				Status:    blank.Status,
			})
		}
		protoPosts = append(protoPosts, &apiv1.GrammarPostCard{
			NotebookId: post.NotebookID,
			EntryId:    post.EntryID,
			Title:      post.Title,
			PostText:   post.Content,
			Blanks:     protoBlanks,
		})
	}
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartGrammarQuizResponse{Posts: protoPosts}), nil
}

// grammarGradeConcurrency caps how many blanks in one request are graded at
// once. Most answers grade deterministically (no network); this only bounds the
// few that fall back to the LLM so a wrong-heavy post can't fan out unbounded
// model calls. The client already submits a post in small chunks, so this is a
// second, per-request ceiling.
const grammarGradeConcurrency = 8

// SubmitGrammarPost grades the blanks in one request and records each result.
// Blanks are graded concurrently (bounded) so the handful that need an LLM call
// don't serialize; results keep request order.
func (h *QuizHandler) SubmitGrammarPost(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitGrammarPostRequest],
) (*connect.Response[apiv1.SubmitGrammarPostResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()
	userID, _ := auth.UserIDFromContext(ctx)

	// Resolve every blank's context up front (guarded map read).
	ctxs := make([]grammarBlankCtx, len(answers))
	for i, a := range answers {
		h.mu.Lock()
		bc, ok := h.grammarStore[a.GetNoteId()]
		h.mu.Unlock()
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("grammar blank %d not found", a.GetNoteId()))
		}
		ctxs[i] = bc
	}

	// Grade concurrently — the only step that may hit the LLM. Deterministic
	// matches return instantly; the bounded group keeps a wrong-heavy post from
	// fanning out unbounded model calls. Grading touches no shared state.
	grades := make([]quiz.GradeResult, len(answers))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(grammarGradeConcurrency)
	for i, a := range answers {
		if a.GetIsSkipped() {
			grades[i] = skippedGradeResult()
			continue
		}
		group.Go(func() error {
			g, err := h.svc.GradeGrammarBlank(groupCtx, ctxs[i].content, ctxs[i].blank, a.GetAnswer(), a.GetResponseTimeMs())
			if err != nil {
				return fmt.Errorf("grade grammar blank: %w", err)
			}
			grades[i] = g
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Save sequentially — writes go through the learning repository, so keep
	// them serialized and in order.
	results := make([]*apiv1.GrammarBlankResult, len(answers))
	for i, a := range answers {
		bc := ctxs[i]
		if err := h.svc.SaveGrammarBlank(ctx, userID, bc.notebookID, bc.blank.SenseID, grades[i], a.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save grammar result: %w", err))
		}
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(userID, bc.notebookID, bc.blank.SenseID, bc.blank.SenseID, notebook.QuizTypeGrammar)
		// For a wrong answer, surface the grader's critique of THIS answer as the
		// assessment; the authored note stays in reason. Correct/skipped answers
		// carry no assessment.
		assessment := ""
		if !a.GetIsSkipped() && !grades[i].Correct {
			assessment = grades[i].Reason
		}
		results[i] = &apiv1.GrammarBlankResult{
			NoteId:         a.GetNoteId(),
			SenseId:        bc.blank.SenseID,
			Correct:        grades[i].Correct,
			CorrectAnswer:  bc.blank.Correct,
			Incorrect:      bc.blank.Incorrect,
			Reason:         bc.blank.Reason,
			Category:       bc.blank.Category,
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			Assessment:     assessment,
		}
	}
	return connect.NewResponse(&apiv1.SubmitGrammarPostResponse{Results: results}), nil
}

// ListGrammarMistakes lists a journal's grammar mistakes — both due and already
// excluded — for the standalone mistake review page (no live quiz session).
func (h *QuizHandler) ListGrammarMistakes(
	ctx context.Context,
	req *connect.Request[apiv1.ListGrammarMistakesRequest],
) (*connect.Response[apiv1.ListGrammarMistakesResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	mistakes, err := h.svc.LoadGrammarMistakes(userID, req.Msg.GetNotebookId(), req.Msg.GetSectionTitles())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load grammar mistakes: %w", err))
	}
	protoMistakes := make([]*apiv1.GrammarMistake, 0, len(mistakes))
	for _, m := range mistakes {
		protoMistakes = append(protoMistakes, &apiv1.GrammarMistake{
			SenseId:    m.SenseID,
			EntryId:    m.EntryID,
			Title:      m.Title,
			Incorrect:  m.Incorrect,
			Correct:    m.Correct,
			Category:   m.Category,
			Reason:     m.Reason,
			Status:     m.Status,
			IsExcluded: m.IsExcluded,
		})
	}
	return connect.NewResponse(&apiv1.ListGrammarMistakesResponse{Mistakes: protoMistakes}), nil
}

// grammarMistakeCardInfo builds the canonical CardInfo that addresses one
// grammar correction's learning-history slot by its stable (notebook_id,
// sense_id): the flat "journal"-titled grammar bucket keyed by the correction
// id — the SAME slot the live grammar quiz's per-blank Exclude uses via
// resolveCardInfo, so excluding here removes the mistake from the live quiz and
// the Relearn pool (invariant L2 / U1-U2).
func grammarMistakeCardInfo(notebookID, senseID string) quiz.CardInfo {
	return quiz.CardInfo{
		NotebookName: notebookID,
		StoryTitle:   notebook.JournalStoryTitle,
		Expression:   senseID,
		ID:           senseID,
	}
}

// ExcludeGrammarMistake sets the grammar skipped_at marker for one correction,
// addressed by its stable (notebook_id, sense_id). It reuses the same
// SkipWord / SetSkippedAt path (and EnsureExpressionStubForSkip when the
// correction has no log yet) every other card's Exclude uses.
func (h *QuizHandler) ExcludeGrammarMistake(
	ctx context.Context,
	req *connect.Request[apiv1.ExcludeGrammarMistakeRequest],
) (*connect.Response[apiv1.ExcludeGrammarMistakeResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	info := grammarMistakeCardInfo(req.Msg.GetNotebookId(), req.Msg.GetSenseId())
	if err := h.svc.SkipWord(userID, info, "", []notebook.QuizType{notebook.QuizTypeGrammar}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("exclude grammar mistake: %w", err))
	}
	return connect.NewResponse(&apiv1.ExcludeGrammarMistakeResponse{}), nil
}

// ResumeGrammarMistake clears the grammar skipped_at marker for one correction,
// making it due again in the live quiz and the Relearn pool.
func (h *QuizHandler) ResumeGrammarMistake(
	ctx context.Context,
	req *connect.Request[apiv1.ResumeGrammarMistakeRequest],
) (*connect.Response[apiv1.ResumeGrammarMistakeResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	info := grammarMistakeCardInfo(req.Msg.GetNotebookId(), req.Msg.GetSenseId())
	if err := h.svc.ResumeWord(userID, info, []notebook.QuizType{notebook.QuizTypeGrammar}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume grammar mistake: %w", err))
	}
	return connect.NewResponse(&apiv1.ResumeGrammarMistakeResponse{}), nil
}
