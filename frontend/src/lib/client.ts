const BASE_URL = "http://localhost:8080";

export interface NotebookSummary {
  notebookId: string;
  name: string;
  reviewCount: number;
}

export interface GetQuizOptionsResponse {
  notebooks: NotebookSummary[];
}

export interface StartQuizRequest {
  notebookIds: string[];
  includeUnstudied: boolean;
}

interface FlashcardResponse {
  noteId: string;
  entry: string;
  examples: { text: string; speaker: string }[];
}

export interface StartQuizResponse {
  flashcards: FlashcardResponse[];
}

async function rpc<Req, Res>(method: string, request: Req): Promise<Res> {
  const response = await fetch(
    `${BASE_URL}/quiz.v1.QuizService/${method}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    }
  );

  if (!response.ok) {
    throw new Error(`RPC ${method} failed: ${response.status}`);
  }

  return response.json();
}

export async function getQuizOptions(): Promise<GetQuizOptionsResponse> {
  return rpc<Record<string, never>, GetQuizOptionsResponse>("GetQuizOptions", {});
}

export async function startQuiz(
  request: StartQuizRequest
): Promise<StartQuizResponse> {
  return rpc<StartQuizRequest, StartQuizResponse>("StartQuiz", request);
}

export interface SubmitAnswerRequest {
  noteId: string;
  answer: string;
  responseTimeMs: string;
}

export interface SubmitAnswerResponse {
  correct: boolean;
  meaning: string;
  reason: string;
}

export async function submitAnswer(
  request: SubmitAnswerRequest
): Promise<SubmitAnswerResponse> {
  return rpc<SubmitAnswerRequest, SubmitAnswerResponse>("SubmitAnswer", request);
}
