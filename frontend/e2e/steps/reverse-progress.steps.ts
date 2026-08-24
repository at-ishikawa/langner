import { expect, type Locator, type Page } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// Captured within a single scenario. workers:1 + sequential scenario execution
// make a module-level value safe here; every scenario captures before it
// asserts, so a stale value can never leak across scenarios.
let capturedReverseCount: number | null = null;

// The quiz start screen renders each notebook as a Chakra v3 Checkbox.Root
// <label> whose visible text is the notebook name followed by the active
// mode's due count (name on the left, count on the right — see
// src/app/quiz/page.tsx). We match the row by name and read the trailing
// integer as the count. Mirrors the checkboxLabel hook in quiz.steps.ts.
function notebookRow(page: Page, notebook: string): Locator {
  return page
    .locator('label[data-scope="checkbox"][data-part="root"]')
    .filter({ hasText: notebook });
}

async function readReverseCount(page: Page, notebook: string): Promise<number> {
  const row = notebookRow(page, notebook);
  await expect(row).toBeVisible();
  const text = (await row.innerText()).trim();
  const match = text.match(/(\d+)\s*$/);
  if (!match) {
    throw new Error(`No review count found in notebook row text: "${text}"`);
  }
  return Number(match[1]);
}

When(
  "I capture the reverse review count for the {string} notebook",
  async ({ page }, notebook: string) => {
    capturedReverseCount = await readReverseCount(page, notebook);
  },
);

Then(
  "the reverse review count for the {string} notebook dropped by one",
  async ({ page }, notebook: string) => {
    if (capturedReverseCount === null) {
      throw new Error("No reverse review count was captured before asserting.");
    }
    const expected = capturedReverseCount - 1;
    // The count comes from a fresh getQuizOptions call on page load, so poll
    // until the row settles on the expected post-answer value.
    await expect
      .poll(() => readReverseCount(page, notebook))
      .toBe(expected);
  },
);
