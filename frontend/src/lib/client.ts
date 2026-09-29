import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { QuizService } from "@/gen-protos/api/v1/quiz_pb";
import { NotebookService } from "@/gen-protos/api/v1/notebook_pb";
import { AnalyticsService } from "@/gen-protos/api/v1/analytics_pb";
import { getAccessToken } from "./authToken";
import { redirectToSignIn } from "./auth";
import { tryRefreshSession } from "./refresh";

// authFetch attaches the bearer access token to every RPC. On a 401
// (missing/expired token) it first tries a silent refresh (exchange the
// stored refresh token for a new access token) and retries the request; only if
// that fails does it fall back to a redirect through Google. Auth is a header,
// not a cookie, so no `credentials: "include"`.
const authFetch: typeof fetch = async (input, init) => {
  const send = (token: string | null): Promise<Response> => {
    const headers = new Headers(init?.headers);
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(input, { ...init, headers });
  };
  let res = await send(getAccessToken());
  if (res.status === 401 && typeof window !== "undefined") {
    if (await tryRefreshSession()) {
      res = await send(getAccessToken());
    }
    if (res.status === 401) {
      redirectToSignIn();
    }
  }
  return res;
};

const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080",
  useBinaryFormat: process.env.NEXT_PUBLIC_CONNECT_JSON !== "true",
  fetch: authFetch,
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
} from "@/gen-protos/api/v1/analytics_pb";

export { Granularity, TrendGroupBy } from "@/gen-protos/api/v1/analytics_pb";
