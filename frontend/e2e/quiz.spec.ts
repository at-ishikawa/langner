import { test, expect } from "@playwright/test";

const GET_QUIZ_OPTIONS_URL = /GetQuizOptions/;
const START_QUIZ_URL = /StartQuiz/;
const SUBMIT_ANSWER_URL = /SubmitAnswer/;

const CONNECT_JSON_CONTENT_TYPE = "application/connect+json";

const mockNotebooks = [
  { notebookId: "english-phrases", name: "English Phrases", reviewCount: 2 },
];

const mockFlashcards = [
  {
    noteId: "1",
    entry: "break the ice",
    examples: [{ text: "She told a joke to break the ice.", speaker: "Alice" }],
  },
  {
    noteId: "2",
    entry: "lose one's temper",
    examples: [],
  },
];

test("shows notebooks and starts quiz", async ({ page }) => {
  let startQuizBody: unknown;

  await page.route(GET_QUIZ_OPTIONS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ notebooks: mockNotebooks }),
    });
  });

  await page.route(START_QUIZ_URL, async (route) => {
    startQuizBody = JSON.parse((await route.request().postData()) ?? "{}");
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ flashcards: mockFlashcards }),
    });
  });

  await page.goto("/");

  // Wait for the GetQuizOptions response to be intercepted
  await page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });

  await expect(page.getByText("English Phrases")).toBeVisible();

  await page.getByRole("checkbox", { name: /English Phrases/ }).click();

  const startButton = page.getByRole("button", { name: "Start" });
  await expect(startButton).toBeEnabled();
  await startButton.click();

  await page.waitForURL("/quiz");

  expect(startQuizBody).toMatchObject({
    notebookIds: ["english-phrases"],
  });
});

test("completes full quiz flow", async ({ page }) => {
  let submitAnswerCallCount = 0;

  await page.route(GET_QUIZ_OPTIONS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ notebooks: mockNotebooks }),
    });
  });

  await page.route(START_QUIZ_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ flashcards: mockFlashcards }),
    });
  });

  await page.route(SUBMIT_ANSWER_URL, async (route) => {
    submitAnswerCallCount++;
    const isFirstCard = submitAnswerCallCount === 1;
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify(
        isFirstCard
          ? {
              correct: true,
              meaning: "to initiate social interaction",
              reason: "The answer captures the core meaning",
            }
          : {
              correct: false,
              meaning: "to initiate social interaction",
              reason: "The answer does not match",
            }
      ),
    });
  });

  await page.goto("/");

  // Wait for the GetQuizOptions response to be intercepted
  await page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });

  await expect(page.getByText("English Phrases")).toBeVisible();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click();
  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz");

  await expect(page.getByRole("heading", { name: "break the ice" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("start a conversation");
  await page.getByRole("button", { name: "Submit" }).click();

  expect(submitAnswerCallCount).toBe(1);

  await expect(page.getByText(/Correct|Incorrect/)).toBeVisible();

  await page.getByRole("button", { name: "Next" }).click();

  await expect(page.getByRole("heading", { name: "lose one's temper" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("get angry");
  await page.getByRole("button", { name: "Submit" }).click();

  expect(submitAnswerCallCount).toBe(2);

  await expect(page.getByText(/Correct|Incorrect/)).toBeVisible();

  await page.getByRole("button", { name: "See Results" }).click();

  await page.waitForURL("/quiz/complete");

  await expect(page.getByText("Session Complete")).toBeVisible();
  await expect(page.getByText(/Total: 2 words/)).toBeVisible();
});
