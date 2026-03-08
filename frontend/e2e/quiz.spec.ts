import { test, expect } from "@playwright/test";

const GET_QUIZ_OPTIONS_URL = /GetQuizOptions/;
const START_QUIZ_URL = /StartQuiz/;
const SUBMIT_ANSWER_URL = /SubmitAnswer/;
const START_FREEFORM_QUIZ_URL = /StartFreeformQuiz/;
const SUBMIT_FREEFORM_ANSWER_URL = /SubmitFreeformAnswer/;

const CONNECT_JSON_CONTENT_TYPE = "application/json";

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

  // Capture browser console errors for diagnostics
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      console.log("BROWSER ERROR:", msg.text());
    }
  });
  page.on("pageerror", (err) => {
    console.log("PAGE ERROR:", err.message);
  });

  await page.route(GET_QUIZ_OPTIONS_URL, async (route) => {
    const reqContentType = route.request().headers()["content-type"] ?? "none";
    console.log("GetQuizOptions request Content-Type:", reqContentType);
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

  const getOptionsPromise = page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });
  await page.goto("/");

  // Wait for the GetQuizOptions response to be intercepted
  await getOptionsPromise;

  // Diagnostic: log what the page renders after GetQuizOptions response
  const bodyText = await page.locator("body").innerText().catch(() => "failed to get text");
  console.log("Page body text after GetQuizOptions:", bodyText.substring(0, 1000));

  await expect(page.getByText("English Phrases")).toBeVisible();

  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });

  const startButton = page.getByRole("button", { name: "Start" });
  await expect(startButton).toBeEnabled();
  await startButton.click();

  await page.waitForURL("/quiz/standard");

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

  const getOptionsPromise = page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });
  await page.goto("/");

  // Wait for the GetQuizOptions response to be intercepted
  await getOptionsPromise;

  await expect(page.getByText("English Phrases")).toBeVisible();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });
  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz/standard");

  await expect(page.getByRole("heading", { name: "break the ice" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("start a conversation");
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText(/Correct|Incorrect/)).toBeVisible();
  expect(submitAnswerCallCount).toBe(1);

  await page.getByRole("button", { name: "Next", exact: true }).click();

  await expect(page.getByRole("heading", { name: "lose one's temper" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("get angry");
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText(/Correct|Incorrect/)).toBeVisible();
  expect(submitAnswerCallCount).toBe(2);

  await page.getByRole("button", { name: "See Results" }).click();

  await page.waitForURL("/quiz/complete");

  await expect(page.getByText("Session Complete")).toBeVisible();
  await expect(page.getByText(/Total: 2 words/)).toBeVisible();
});

test("completes freeform quiz flow and shows results", async ({ page }) => {
  await page.route(GET_QUIZ_OPTIONS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ notebooks: mockNotebooks }),
    });
  });

  await page.route(START_FREEFORM_QUIZ_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ wordCount: 10 }),
    });
  });

  let submitCount = 0;
  await page.route(SUBMIT_FREEFORM_ANSWER_URL, async (route) => {
    submitCount++;
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify(
        submitCount === 1
          ? {
              correct: true,
              word: "hit the hay",
              meaning: "to go to bed",
              reason: "The answer captures the core meaning",
              notebookName: "English Phrases",
            }
          : {
              correct: false,
              word: "under the weather",
              meaning: "to feel sick",
              reason: "The answer does not match",
              notebookName: "English Phrases",
            }
      ),
    });
  });

  const getOptionsPromise = page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });
  await page.goto("/");
  await getOptionsPromise;

  // Select freeform quiz type
  await page.getByText("Freeform").click();
  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz/freeform");

  // Submit first answer
  await page.getByPlaceholder("e.g., hit the hay").fill("hit the hay");
  await page.getByPlaceholder("e.g., to go to bed; to sleep").fill("to go to sleep");
  await page.getByRole("button", { name: "Check Answer" }).click();

  await expect(page.getByText(/Correct!/)).toBeVisible();

  // Go to next word
  await page.getByRole("button", { name: "Next Word" }).click();

  // Submit second answer
  await page.getByPlaceholder("e.g., hit the hay").fill("under the weather");
  await page.getByPlaceholder("e.g., to go to bed; to sleep").fill("to be happy");
  await page.getByRole("button", { name: "Check Answer" }).click();

  await expect(page.getByText(/Incorrect/)).toBeVisible();

  // Navigate to results
  await page.getByRole("button", { name: "See Results" }).click();

  await page.waitForURL("/quiz/complete");

  await expect(page.getByText("Session Complete")).toBeVisible();
  await expect(page.getByText(/Total: 2 words/)).toBeVisible();
  await expect(page.getByText(/Correct: 1/)).toBeVisible();
  await expect(page.getByText(/Incorrect: 1/)).toBeVisible();
});
