import { createClient } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { QuizService } from "@/gen-protos/api/v1/quiz_pb";
import { NotebookService } from "@/gen-protos/api/v1/notebook_pb";
import { AnalyticsService } from "@/gen-protos/api/v1/analytics_pb";

// messageForRpcError turns a grading RPC failure into an actionable, user-facing
// message. The backend maps provider failures to specific Connect codes (see
// server.mapGradeError): quota/credits -> ResourceExhausted; invalid or missing
// key -> Unauthenticated / FailedPrecondition. Everything else falls back to a
// caller-supplied generic message so unrelated failures read sensibly.
export function messageForRpcError(err: unknown, fallback: string): string {
  if (err instanceof ConnectError) {
    switch (err.code) {
      case Code.ResourceExhausted:
        return "Your LLM API key has no remaining credits. Update it in Settings.";
      case Code.Unauthenticated:
        return "Your LLM API key is invalid. Update it in Settings.";
      case Code.FailedPrecondition:
        return "No LLM API key configured. Add one in Settings to grade answers.";
    }
    // The backend's mapped messages are already user-facing; prefer them when present.
    if (err.rawMessage) return err.rawMessage;
  }
  return fallback;
}

const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080",
  useBinaryFormat: process.env.NEXT_PUBLIC_CONNECT_JSON !== "true",
  // Send the HTTP-only session cookie with every RPC so the backend's auth
  // interceptor can authenticate the request cross-origin (frontend 3100 ↔
  // backend 8080). Requires credentialed CORS on the server.
  fetch: (input, init) => fetch(input, { ...init, credentials: "include" }),
});

export const quizClient = createClient(QuizService, transport);
export const notebookClient = createClient(NotebookService, transport);
export const analyticsClient = createClient(AnalyticsService, transport);

export type {
  NotebookSummary,
  NotebookSectionSummary,
  StartQuizRequest,
  StartQuizResponse,
  GetQuizOptionsResponse,
  Flashcard,
  Example,
  SubmitAnswerRequest,
  SubmitAnswerResponse,
  StartReverseQuizRequest,
  StartReverseQuizResponse,
  ReverseFlashcard,
  ContextSentence,
  SubmitReverseAnswerRequest,
  SubmitReverseAnswerResponse,
  StartFreeformQuizRequest,
  StartFreeformQuizResponse,
  SubmitFreeformAnswerRequest,
  SubmitFreeformAnswerResponse,
  OverrideAnswerRequest,
  OverrideAnswerResponse,
  UndoOverrideAnswerRequest,
  UndoOverrideAnswerResponse,
  SkipWordRequest,
  SkipWordResponse,
  ResumeWordRequest,
  ResumeWordResponse,
  GraphPrompt,
  GraphNode,
  GraphEdge,
  StartRelearnQuizRequest,
  StartRelearnQuizResponse,
  RelearnCard,
  SubmitRelearnAnswerRequest,
  SubmitRelearnAnswerResponse,
  RelearnContextScene,
  RelearnConversationLine,
  BatchSubmitRelearnAnswersRequest,
  BatchSubmitRelearnAnswersResponse,
  GrammarPostCard,
  GrammarBlank,
  StartGrammarQuizRequest,
  StartGrammarQuizResponse,
  SubmitGrammarPostRequest,
  SubmitGrammarPostResponse,
  GrammarBlankAnswer,
  GrammarBlankResult,
  GrammarMistake,
  ListGrammarMistakesRequest,
  ListGrammarMistakesResponse,
  ExcludeGrammarMistakeRequest,
  ResumeGrammarMistakeRequest,
} from "@/gen-protos/api/v1/quiz_pb";

export { QuizType } from "@/gen-protos/api/v1/quiz_pb";

export type {
  GetNotebookDetailResponse,
  StoryEntry,
  StoryScene,
  StoryMetadata,
  Conversation,
  NotebookWord,
  LearningLogEntry,
  ExportNotebookPDFResponse,
  LookupWordRequest,
  LookupWordResponse,
  WordDefinition,
  RegisterDefinitionRequest,
  RegisterDefinitionResponse,
  DeleteDefinitionRequest,
  DeleteDefinitionResponse,
  EtymologyOriginPart,
  EtymologyOriginForm,
  EtymologyDefinition,
  EtymologyMeaningGroup,
  GetEtymologyNotebookRequest,
  GetEtymologyNotebookResponse,
  SemanticConcept,
  SemanticConceptMember,
  ConceptRelation,
} from "@/gen-protos/api/v1/notebook_pb";

export type {
  AnalyticsFilters,
  GetDailySummariesRequest,
  GetDailySummariesResponse,
  DailySummary,
  GetDayDetailRequest,
  GetDayDetailResponse,
  WrongWord,
  RelatedGroup,
  GetWordHistoryRequest,
  GetWordHistoryResponse,
  AttemptEntry,
  GetTrendsRequest,
  GetTrendsResponse,
  TrendBucket,
  TrendSeries,
  TrendsSummary,
  BacklogSnapshot,
} from "@/gen-protos/api/v1/analytics_pb";

export { Granularity, TrendGroupBy } from "@/gen-protos/api/v1/analytics_pb";
