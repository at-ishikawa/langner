package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
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
	_ context.Context,
	req *connect.Request[apiv1.StartGrammarQuizRequest],
) (*connect.Response[apiv1.StartGrammarQuizResponse], error) {
	var posts []quiz.GrammarPost
	for _, notebookID := range req.Msg.GetNotebookIds() {
		loaded, err := h.svc.LoadGrammarPosts(notebookID)
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
				Line:      int32(blank.Line),
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

// SubmitGrammarPost grades every blank the user filled for one post at once and
// records each result. Grading runs sequentially because all blanks in a post
// write the same notebook's learning-notes file.
func (h *QuizHandler) SubmitGrammarPost(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitGrammarPostRequest],
) (*connect.Response[apiv1.SubmitGrammarPostResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()
	results := make([]*apiv1.GrammarBlankResult, 0, len(answers))
	for _, a := range answers {
		h.mu.Lock()
		bc, ok := h.grammarStore[a.GetNoteId()]
		h.mu.Unlock()
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("grammar blank %d not found", a.GetNoteId()))
		}

		var grade quiz.GradeResult
		if a.GetIsSkipped() {
			grade = skippedGradeResult()
		} else {
			var err error
			grade, err = h.svc.GradeGrammarBlank(ctx, bc.content, bc.blank, a.GetAnswer(), a.GetResponseTimeMs())
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade grammar blank: %w", err))
			}
		}
		if err := h.svc.SaveGrammarBlank(ctx, bc.notebookID, bc.blank.SenseID, grade, a.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save grammar result: %w", err))
		}

		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(bc.notebookID, bc.blank.SenseID, bc.blank.SenseID, notebook.QuizTypeGrammar)
		results = append(results, &apiv1.GrammarBlankResult{
			NoteId:         a.GetNoteId(),
			SenseId:        bc.blank.SenseID,
			Correct:        grade.Correct,
			CorrectAnswer:  bc.blank.Correct,
			Incorrect:      bc.blank.Incorrect,
			Reason:         bc.blank.Reason,
			Category:       bc.blank.Category,
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
		})
	}
	return connect.NewResponse(&apiv1.SubmitGrammarPostResponse{Results: results}), nil
}
