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
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeVisible();
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

// Finds the grammar-format relearn card wherever it falls in the queue —
// the pool can also hold leftover words from earlier scenarios in the same
// window, so this doesn't assume the grammar card is first. Every
// other-format card is skipped past via "Don't Know" (which every Relearn
// format renders) until the grammar card's own struck-through-span input
// (aria-label "Correction for ...", the same labelling the live Grammar
// quiz uses) is found; then it's filled with the exact reference correction,
// which GradeGrammarBlank's deterministic fast path grades correct without
// needing the mock LLM. Leaves the session wherever it lands next so
// "I clear every remaining relearn card" can drain the rest of the pool.
When(
  "I find and fix the grammar relearn card for {string} with {string}",
  async ({ page }, incorrect: string, answer: string) => {
    const submit = page.getByRole("button", { name: "Submit", exact: true });
    const dontKnow = page.getByRole("button", { name: "Don't Know", exact: true });
    const next = page.getByRole("button", { name: "Next", exact: true });
    const grammarInput = page.getByLabel(`Correction for "${incorrect}"`);

    let found = false;
    for (let i = 0; i < 50 && !page.url().includes("/quiz/relearn/complete"); i++) {
      await submit.waitFor({ state: "visible" });
      if (await grammarInput.isVisible().catch(() => false)) {
        found = true;
        await grammarInput.fill(answer);
        await submit.click();
        await expect(page.getByText("✓ Correct")).toBeVisible();
        await next.click();
        break;
      }
      // Some other-format card — skip past it (wrong is fine; it just
      // requeues to the back) and keep looking.
      await dontKnow.click();
      await next.waitFor({ state: "visible" });
      await next.click();
      await page.waitForFunction(
        () =>
          location.pathname.includes("/quiz/relearn/complete") ||
          !!document.querySelector("input"),
        undefined,
        { timeout: 15000 },
      );
    }
    expect(found, `grammar relearn card for "${incorrect}" never appeared in the pool`).toBe(
      true,
    );
  },
);

// covers route: /quiz/relearn/complete
Then("I should be on the Relearn Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/relearn\/complete/);
  await expect(page.getByRole("heading", { name: "Relearn complete" })).toBeVisible();
});
