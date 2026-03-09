import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { QuizService } from "@/gen-protos/api/v1/quiz_pb";
import { NotebookService } from "@/gen-protos/api/v1/notebook_pb";

const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080",
  useBinaryFormat: process.env.NEXT_PUBLIC_CONNECT_JSON !== "true",
});

export const quizClient = createClient(QuizService, transport);
export const notebookClient = createClient(NotebookService, transport);

export type {
  NotebookSummary,
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
} from "@/gen-protos/api/v1/quiz_pb";

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
} from "@/gen-protos/api/v1/notebook_pb";
