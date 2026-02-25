import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { QuizService } from "@/gen-protos/api/v1/quiz_pb";

const transport = createConnectTransport({
  baseUrl: "http://localhost:8080",
});

export const quizClient = createClient(QuizService, transport);

export type {
  NotebookSummary,
  StartQuizRequest,
  StartQuizResponse,
  GetQuizOptionsResponse,
  Flashcard,
  Example,
  SubmitAnswerRequest,
  SubmitAnswerResponse,
} from "@/gen-protos/api/v1/quiz_pb";
