import { test, expect } from "@playwright/test";

const GET_QUIZ_OPTIONS_URL = /GetQuizOptions/;
const START_QUIZ_URL = /StartQuiz/;
const BATCH_SUBMIT_ANSWERS_URL = /BatchSubmitAnswers/;
const START_REVERSE_QUIZ_URL = /StartReverseQuiz/;
const START_FREEFORM_QUIZ_URL = /StartFreeformQuiz/;
const SUBMIT_FREEFORM_ANSWER_URL = /SubmitFreeformAnswer/;

const OVERRIDE_ANSWER_URL = /OverrideAnswer/;
const UNDO_OVERRIDE_ANSWER_URL = /UndoOverrideAnswer/;
const SKIP_WORD_URL = /SkipWord/;

const CONNECT_JSON_CONTENT_TYPE = "application/json";

const mockNotebooks = [
  { notebookId: "english-phrases", name: "English Phrases", reviewCount: 2, reverseReviewCount: 1 },
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
  await page.goto("/quiz");

  // Wait for the GetQuizOptions response to be intercepted
  await getOptionsPromise;

  // Diagnostic: log what the page renders after GetQuizOptions response
  const bodyText = await page.locator("body").innerText().catch(() => "failed to get text");
  console.log("Page body text after GetQuizOptions:", bodyText.substring(0, 1000));

  // Select "Standard" mode to reveal notebook selection
  await page.getByText("Standard").click();

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
  let batchCallCount = 0;

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

  // Batch RPC: with feedbackInterval=1, each answer submits a single-item batch.
  // First batch returns correct; second returns incorrect.
  await page.route(BATCH_SUBMIT_ANSWERS_URL, async (route) => {
    batchCallCount++;
    const isFirstBatch = batchCallCount === 1;
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({
        responses: [
          isFirstBatch
            ? {
                correct: true,
                meaning: "to initiate social interaction",
                reason: "The answer captures the core meaning",
              }
            : {
                correct: false,
                meaning: "to initiate social interaction",
                reason: "The answer does not match",
              },
        ],
      }),
    });
  });

  const getOptionsPromise = page.waitForResponse(/GetQuizOptions/, { timeout: 10000 });
  await page.goto("/quiz");

  // Wait for the GetQuizOptions response to be intercepted
  await getOptionsPromise;

  // Select "Standard" mode to reveal notebook selection
  await page.getByText("Standard").click();

  await expect(page.getByText("English Phrases")).toBeVisible();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });

  // Set feedback interval to 1 so feedback shows after each answer
  await page.getByRole("spinbutton").fill("1");

  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz/standard");

  await expect(page.getByRole("heading", { name: "break the ice" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("start a conversation");
  await page.getByRole("button", { name: "Submit" }).click();

  // Batch feedback appears after first answer (interval=1)
  await expect(page.getByText(/Correct: 1/)).toBeVisible();
  expect(batchCallCount).toBe(1);

  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByRole("heading", { name: "lose one's temper" })).toBeVisible();

  await page.getByPlaceholder("Type your answer").fill("get angry");
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText(/Incorrect: 1/)).toBeVisible();
  expect(batchCallCount).toBe(2);

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
  await page.goto("/quiz");
  await getOptionsPromise;

  // Select freeform quiz type
  await page.getByText("Freeform").click();
  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz/freeform");

  // Submit first answer
  await page.getByPlaceholder("e.g., hit the hay").fill("hit the hay");
  await page.getByPlaceholder("e.g., to go to bed; to sleep").fill("to go to sleep");
  await page.getByRole("button", { name: "Check Answer" }).click();

  await expect(page.getByText(/^[✓✗] (?:Correct|Incorrect)/)).toBeVisible();

  // Go to next word
  await page.getByRole("button", { name: "Next Word" }).click();

  // Submit second answer
  await page.getByPlaceholder("e.g., hit the hay").fill("under the weather");
  await page.getByPlaceholder("e.g., to go to bed; to sleep").fill("to be happy");
  await page.getByRole("button", { name: "Check Answer" }).click();

  await expect(page.getByText(/^[✓✗] (?:Correct|Incorrect)/)).toBeVisible();

  // Navigate to results
  await page.getByRole("button", { name: "See Results" }).click();

  await page.waitForURL("/quiz/complete");

  await expect(page.getByText("Session Complete")).toBeVisible();
  await expect(page.getByText(/Total: 2 words/)).toBeVisible();
  await expect(page.getByText(/Correct: 1/)).toBeVisible();
  await expect(page.getByText(/Incorrect: 1/)).toBeVisible();
});

const mockReverseFlashcards = [
  {
    noteId: "1",
    meaning: "to initiate social interaction",
    contexts: [{ context: "She told a joke to break the ice.", maskedContext: "She told a joke to ___." }],
    notebookName: "English Phrases",
    storyTitle: "",
    sceneTitle: "",
  },
];

test("standard quiz starts correctly after a reverse quiz", async ({ page }) => {
  await page.route(GET_QUIZ_OPTIONS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ notebooks: mockNotebooks }),
    });
  });

  await page.route(START_REVERSE_QUIZ_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ flashcards: mockReverseFlashcards }),
    });
  });

  await page.route(START_QUIZ_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({ flashcards: mockFlashcards }),
    });
  });

  // Step 1: Start a reverse quiz to set quizType to "reverse" in the store
  const getOptionsPromise = page.waitForResponse(GET_QUIZ_OPTIONS_URL, { timeout: 10000 });
  await page.goto("/quiz");
  await getOptionsPromise;

  await page.getByText("Reverse").click();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });
  await page.getByRole("button", { name: "Start" }).click();

  await page.waitForURL("/quiz/reverse");

  // Step 2: Navigate back using client-side navigation to preserve Zustand store state
  // Using browser back button keeps the in-memory store (unlike page.goto which reloads)
  await page.goBack();
  await page.waitForURL("/quiz");

  // Select "Standard" mode — no mode is pre-selected after navigating back
  await page.getByText("Standard").click();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });
  await page.getByRole("button", { name: "Start" }).click();

  // Should navigate to /quiz/standard, NOT redirect to /
  await page.waitForURL("/quiz/standard");
  await expect(page.getByRole("heading", { name: "break the ice" })).toBeVisible();
});

test("override answer in standard quiz feedback", async ({ page }) => {
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
      body: JSON.stringify({ flashcards: [mockFlashcards[0]] }),
    });
  });

  await page.route(BATCH_SUBMIT_ANSWERS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({
        responses: [
          {
            correct: true,
            meaning: "to initiate social interaction",
            reason: "The answer captures the core meaning",
            nextReviewDate: "2027-06-15",
            learnedAt: "2026-03-16T00:00:00Z",
          },
        ],
      }),
    });
  });

  await page.route(OVERRIDE_ANSWER_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({
        nextReviewDate: "2027-06-20",
        originalQuality: 5,
        originalStatus: "understood",
        originalIntervalDays: 10,
        originalEasinessFactor: 2.5,
      }),
    });
  });

  await page.route(UNDO_OVERRIDE_ANSWER_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({
        correct: true,
        nextReviewDate: "2027-06-15",
      }),
    });
  });

  const getOptionsPromise = page.waitForResponse(GET_QUIZ_OPTIONS_URL, { timeout: 10000 });
  await page.goto("/quiz");
  await getOptionsPromise;

  await page.getByText("Standard").click();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });
  await page.getByRole("button", { name: "Start" }).click();
  await page.waitForURL("/quiz/standard");

  // Submit answer
  await page.getByPlaceholder("Type your answer").fill("start a conversation");
  await page.getByRole("button", { name: "Submit" }).click();

  // Final card in batch — batch feedback shows "Correct: 1"
  await expect(page.getByText(/Correct: 1/)).toBeVisible();

  // Click "Mark as Incorrect" on the result card
  await page.getByRole("button", { name: "Mark as Incorrect" }).click();

  // Verify "(overridden)" or "Marked as" label appears
  await expect(page.getByText(/overridden/)).toBeVisible();

  // Click "Undo override" to restore original state
  await page.getByText("Undo override").click();

  // Verify override is cleared
  await expect(page.getByText(/overridden/)).not.toBeVisible();
});

test("skip word in standard quiz feedback", async ({ page }) => {
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
      body: JSON.stringify({ flashcards: [mockFlashcards[0]] }),
    });
  });

  await page.route(BATCH_SUBMIT_ANSWERS_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({
        responses: [
          {
            correct: true,
            meaning: "to initiate social interaction",
            reason: "The answer captures the core meaning",
            nextReviewDate: "2027-06-15",
            learnedAt: "2026-03-16T00:00:00Z",
          },
        ],
      }),
    });
  });

  await page.route(SKIP_WORD_URL, async (route) => {
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": CONNECT_JSON_CONTENT_TYPE },
      body: JSON.stringify({}),
    });
  });

  const getOptionsPromise = page.waitForResponse(GET_QUIZ_OPTIONS_URL, { timeout: 10000 });
  await page.goto("/quiz");
  await getOptionsPromise;

  await page.getByText("Standard").click();
  await page.getByRole("checkbox", { name: /English Phrases/ }).click({ force: true });
  await page.getByRole("button", { name: "Start" }).click();
  await page.waitForURL("/quiz/standard");

  // Submit answer
  await page.getByPlaceholder("Type your answer").fill("start a conversation");
  await page.getByRole("button", { name: "Submit" }).click();

  // Batch feedback shows correct count
  await expect(page.getByText(/Correct: 1/)).toBeVisible();

  // Click "Exclude" on the result card
  await page.getByRole("button", { name: "Exclude" }).click();

  // Verify the card is moved to the Excluded section
  await expect(page.getByText(/Excluded from Quizzes \(1\)/)).toBeVisible();

  // Verify Resume button appears on the excluded card
  await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();
});

// Review date display and change functionality was removed from the per-question feedback screen
