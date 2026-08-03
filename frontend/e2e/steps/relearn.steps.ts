import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// The Relearn tab switches content in place within the Quiz hub (no navigation).
When("I open the Relearn Quiz", async ({ page }) => {
  await page.goto("/quiz");
  await page.getByText("Relearn", { exact: true }).click();
  await expect(page.getByText("Look back over the last:")).toBeVisible();
});

Then("I see words to relearn", async ({ page }) => {
  await expect(page.getByText(/to relearn/i)).toBeVisible();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeEnabled();
});

// covers route: /quiz/relearn/session
When("I start the relearn session", async ({ page }) => {
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page).toHaveURL(/\/quiz\/relearn\/session/);
});

Then("I see a relearn card", async ({ page }) => {
  await expect(page.getByText(/words? left/i)).toBeVisible();
  // A screen is either a single-item card (Submit) or a grammar post (the whole
  // entry with inline blanks, drilled progressively like the live grammar quiz).
  const submit = page.getByRole("button", { name: "Submit", exact: true });
  const post = page.getByTestId("relearn-grammar-post");
  await expect(submit.or(post)).toBeVisible();
});

// Loops through the working queue until it empties and the session lands on the
// complete page. Each card mirrors its source quiz type, so the prompt and the
// correct answer differ per card (recognition asks the meaning; reverse asks
// the word; etymology asks the meaning/origin). The mock reverse/etymology
// graders accept only the exact answer, so the loop captures each card's correct
// answer from its feedback (data-testid=relearn-answer) keyed by its prompt
// (data-testid=relearn-prompt) and reuses it on the next encounter: a card is
// answered wrong once (recording the answer), then correct. Recognition cards
// clear on the first non-"wrong" answer. Every card clears within two passes.
//
// exact:true is required on the button names: `next dev` injects a "Open
// Next.js Dev Tools" button whose accessible name contains "Next", which a
// loose name match collides with.
When("I clear every remaining relearn card", async ({ page }) => {
  const answers = new Map<string, string>();
  const submit = page.getByRole("button", { name: "Submit", exact: true });
  const next = page.getByRole("button", { name: "Next", exact: true });
  for (let i = 0; i < 200 && !page.url().includes("/quiz/relearn/complete"); i++) {
    await submit.waitFor({ state: "visible" });
    const prompt = ((await page.getByTestId("relearn-prompt").textContent()) ?? "").trim();
    await page.getByRole("textbox").first().fill(answers.get(prompt) ?? "an attempt");
    await submit.click();
    const answerEl = page.getByTestId("relearn-answer");
    await answerEl.waitFor({ state: "visible" });
    answers.set(prompt, ((await answerEl.textContent()) ?? "").trim());
    await next.click();
    // Wait until either the next card's input mounts or the session navigates
    // to the complete page — avoids racing the blank transition frame.
    await page.waitForFunction(
      () =>
        location.pathname.includes("/quiz/relearn/complete") ||
        !!document.querySelector("input"),
      undefined,
      { timeout: 15000 },
    );
  }
});

// Finds the grammar-format relearn POST wherever it falls in the queue — the
// pool can also hold leftover words from earlier scenarios in the same window,
// so this doesn't assume the grammar post is first. Every other-format card is
// skipped past via "Don't Know" (which every single-item Relearn format
// renders) until the post's due blank — its struck-through-span input
// (aria-label "Correction for ...", the same labelling the live Grammar quiz
// uses) — appears. Grammar relearn now mirrors the live grammar quiz: the whole
// entry is shown once and each blank is graded progressively the moment it is
// committed (blur/Enter), with no per-card Submit; the reference correction hits
// GradeGrammarBlank's deterministic fast path (correct without the mock LLM).
// After the blank turns into a "correct" pill, one "Next" advances past the
// whole post so "I clear every remaining relearn card" can drain the rest.
When(
  "I find and fix the grammar relearn card for {string} with {string}",
  async ({ page }, incorrect: string, answer: string) => {
    const dontKnow = page.getByRole("button", { name: "Don't Know", exact: true });
    const cardNext = page.getByRole("button", { name: "Next", exact: true });
    const grammarInput = page.getByLabel(`Correction for "${incorrect}"`);
    const grammarNext = page.getByTestId("relearn-grammar-next");

    let found = false;
    for (let i = 0; i < 50 && !page.url().includes("/quiz/relearn/complete"); i++) {
      // A screen is up once a grammar post or a single-card prompt is present.
      await page.waitForFunction(
        () =>
          location.pathname.includes("/quiz/relearn/complete") ||
          !!document.querySelector('[data-testid="relearn-grammar-post"]') ||
          !!document.querySelector('[data-testid="relearn-prompt"]'),
        undefined,
        { timeout: 15000 },
      );
      if (page.url().includes("/quiz/relearn/complete")) break;

      if (await grammarInput.isVisible().catch(() => false)) {
        found = true;
        // Commit the fix in place (blur), like the live grammar quiz — no
        // Submit; the blank grades progressively and becomes a "correct" pill.
        await grammarInput.fill(answer);
        await grammarInput.blur();
        await expect(page.getByRole("button", { name: `${incorrect} — correct` })).toBeVisible();
        await grammarNext.click();
        break;
      }
      // Some other-format card — skip past it (wrong is fine; it just requeues
      // to the back) and keep looking.
      await dontKnow.click();
      await cardNext.waitFor({ state: "visible" });
      await cardNext.click();
      await page.waitForFunction(
        () =>
          location.pathname.includes("/quiz/relearn/complete") ||
          !!document.querySelector('[data-testid="relearn-grammar-post"]') ||
          !!document.querySelector("input"),
        undefined,
        { timeout: 15000 },
      );
    }
    expect(found, `grammar relearn post for "${incorrect}" never appeared in the pool`).toBe(
      true,
    );
  },
);

// Navigates the working queue to the grammar POST holding the due blank for
// {string}, then STOPS on it without answering — so a following step can
// exercise "See answers" or the per-blank Exclude control. Two kinds of card
// may sit in front of the target:
//   - a single vocab/etymology card → skipped via its "Don't Know" shortcut (a
//     skip-for-now, not an exclude);
//   - ANOTHER grammar post (a miss pooled by an earlier scenario in the same
//     24h window — the pool is shared, DB state is not reset between scenarios)
//     → it has no "Don't Know", so it is drained by revealing its answers and
//     advancing. Relearn persists nothing, so revealing a distractor post grades
//     nothing to disk and leaves the correction due for its own scenario.
When(
  "I open the grammar relearn post for {string}",
  async ({ page }, incorrect: string) => {
    const dontKnow = page.getByRole("button", { name: "Don't Know", exact: true });
    const cardNext = page.getByRole("button", { name: "Next", exact: true });
    const grammarInput = page.getByLabel(`Correction for "${incorrect}"`);
    const grammarPost = page.getByTestId("relearn-grammar-post");
    const seeAnswers = page.getByRole("button", { name: /See answers/ });
    const closeDetails = page.getByRole("button", { name: "Close details" });
    const grammarNext = page.getByTestId("relearn-grammar-next");

    let found = false;
    for (let i = 0; i < 50 && !page.url().includes("/quiz/relearn/complete"); i++) {
      await page.waitForFunction(
        () =>
          location.pathname.includes("/quiz/relearn/complete") ||
          !!document.querySelector('[data-testid="relearn-grammar-post"]') ||
          !!document.querySelector('[data-testid="relearn-prompt"]'),
        undefined,
        { timeout: 15000 },
      );
      if (page.url().includes("/quiz/relearn/complete")) break;
      if (await grammarInput.isVisible().catch(() => false)) {
        found = true;
        break;
      }
      if (await grammarPost.isVisible().catch(() => false)) {
        // A grammar post that is NOT the target — reveal every blank (marks the
        // unanswered ones incorrect, persisting nothing) then advance past it.
        if (await seeAnswers.isVisible().catch(() => false)) await seeAnswers.click();
        await grammarNext.waitFor({ state: "visible" });
        // The revealed-feedback sheet is pinned to the viewport bottom and can
        // overlap Next; close it first so the click isn't intercepted.
        if (await closeDetails.isVisible().catch(() => false)) await closeDetails.click();
        await grammarNext.click();
      } else {
        await dontKnow.click();
        await cardNext.waitFor({ state: "visible" });
        await cardNext.click();
      }
    }
    expect(found, `grammar relearn post for "${incorrect}" never appeared`).toBe(true);
  },
);

// Reveals every remaining blank on the grammar relearn post. Unanswered blanks
// are graded INCORRECT (a normal miss), never skipped and never excluded.
When(
  "I tap See answers on the grammar relearn post",
  async ({ page }) => {
    await page.getByRole("button", { name: /See answers/ }).click();
  },
);

// The per-blank feedback for an unanswered (revealed) blank must open inline
// (pinned sheet), read INCORRECT, and NEVER say "Excluded".
Then(
  "the grammar relearn feedback shows {string} is incorrect and not excluded",
  async ({ page }, incorrect: string) => {
    const sheet = page.getByTestId("relearn-grammar-feedback");
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText("✗ Incorrect");
    await expect(sheet).not.toContainText(/Excluded/i);
    // The graded blank reads as "incorrect", never "excluded".
    await expect(page.getByRole("button", { name: `${incorrect} — incorrect` })).toBeVisible();
  },
);

// Taps the PER-BLANK Exclude control next to the struck span for {string}. This
// is the ONLY deliberate exclusion — it calls SkipWord (SetSkippedAt) and
// removes the correction from future quizzes (see quiz-ui-invariants.md).
When(
  "I tap Exclude on the grammar relearn blank for {string}",
  async ({ page }, incorrect: string) => {
    await page.getByRole("button", { name: `Exclude "${incorrect}" from quizzes` }).click();
  },
);

// After Exclude, the blank leaves the active post and reads "excluded" — it is
// neither answered nor marked incorrect.
Then(
  "the grammar relearn blank for {string} is excluded",
  async ({ page }, incorrect: string) => {
    const post = page.getByTestId("relearn-grammar-post");
    await expect(post).toContainText(/excluded/i);
    // The blank is no longer an answerable input and is not a graded pill.
    await expect(page.getByLabel(`Correction for "${incorrect}"`)).toHaveCount(0);
    await expect(page.getByRole("button", { name: `${incorrect} — incorrect` })).toHaveCount(0);
  },
);

// covers route: /quiz/relearn/complete
Then("I should be on the Relearn Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/relearn\/complete/);
  await expect(page.getByRole("heading", { name: "Relearn complete" })).toBeVisible();
});
