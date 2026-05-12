import { expect, type Locator, type Page } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, When, Then } = createBdd();

// Chakra v3 wraps Switch.Root and Checkbox.Root in a real <label> tagged with
// data-scope/data-part. That gives us a stable hook even when the visible
// label text appears elsewhere on the page (e.g., the "Turn on Include
// unstudied words" hint shown when the notebook list is empty).
function switchLabel(page: Page, text: string): Locator {
  return page
    .locator('label[data-scope="switch"][data-part="root"]')
    .filter({ hasText: text });
}

function checkboxLabel(page: Page, text: string): Locator {
  return page
    .locator('label[data-scope="checkbox"][data-part="root"]')
    .filter({ hasText: text });
}

When("I choose the {string} quiz mode", async ({ page }, mode: string) => {
  // Each mode card is a clickable Box. The mode title is the first <Text>;
  // the description below it doesn't contain "Standard"/"Reverse"/"Freeform"
  // as a standalone word, so exact-match on the mode name is unique.
  await page.getByText(mode, { exact: true }).first().click();
});

When("I include unstudied words", async ({ page }) => {
  await switchLabel(page, "Include unstudied words").click();
});

// Generic Chakra v3 Switch toggle for any labeled switch on /quiz (currently
// used by "List words missing context" on Reverse mode).
When("I enable {string}", async ({ page }, label: string) => {
  await switchLabel(page, label).click();
});

// Clicking the Chakra v3 Checkbox.Root <label> toggles the hidden input. We
// match the label by its visible text (notebook name + review count) using
// `filter`, which avoids accessible-name flakiness from nested <Text> nodes.
When("I select the {string} notebook", async ({ page }, name: string) => {
  await checkboxLabel(page, name).click();
});

// Starting a quiz navigates to one of the per-mode pages:
//   /quiz/standard, /quiz/reverse, /quiz/freeform,
//   /quiz/etymology-standard, /quiz/etymology-reverse, /quiz/etymology-freeform
When("I start the quiz", async ({ page }) => {
  await page.getByRole("button", { name: /^start$/i }).click();
});

When("I type the answer {string}", async ({ page }, answer: string) => {
  // AnswerInput renders a single <Input> with placeholder "Type your answer"
  // (standard), "Type the word" (reverse), "type the meaning..." (etymology
  // standard), "type the origin..." (etymology reverse). The "answer"
  // placeholder isn't shared, so we fall back to the only visible textbox.
  const input = page.getByRole("textbox").first();
  await input.fill(answer);
});

// Freeform vocabulary mode has TWO separate fields: an <Input> for the word
// and a <Textarea> for the meaning. The labels are <Text>, not <label>
// elements, so we scope by the example placeholders that each page hard-codes
// for those inputs.
When("I type the word {string}", async ({ page }, word: string) => {
  await page.getByPlaceholder(/e\.g\., hit the hay/i).fill(word);
});

When("I type the origin {string}", async ({ page }, originText: string) => {
  // /quiz/etymology-freeform: Origin <Input> with placeholder "e.g., spect".
  await page.getByPlaceholder(/e\.g\., spect/i).fill(originText);
});

When("I type the meaning {string}", async ({ page }, meaning: string) => {
  // Two pages use a "Meaning" input: vocabulary freeform (placeholder "e.g.,
  // to go to bed; to sleep") and etymology freeform (placeholder "e.g., to
  // look or see"). Each scenario only exposes one of them at a time, so we
  // try both.
  const vocab = page.getByPlaceholder(/e\.g\., to go to bed/i);
  if (await vocab.first().isVisible().catch(() => false)) {
    await vocab.first().fill(meaning);
    return;
  }
  await page.getByPlaceholder(/e\.g\., to look or see/i).fill(meaning);
});

When("I submit my answer", async ({ page }) => {
  // The submit button is rendered as "Submit" (AnswerInput, etymology-freeform)
  // or "Check Answer" (vocabulary freeform). Match either one.
  await page
    .getByRole("button", { name: /^(submit|check answer)$/i })
    .first()
    .click();
});

// AnswerInput renders a secondary "Don't Know" button next to Submit when its
// onSkip handler is wired (Standard, Reverse, Etymology Standard/Reverse).
When("I skip the card", async ({ page }) => {
  await page.getByRole("button", { name: /^don'?t know$/i }).first().click();
});

// Fail the next BatchSubmitAnswers RPC exactly once so the Standard quiz
// surfaces its "Retry grading" button. Subsequent calls go through as normal,
// letting the test resume after the retry.
Given(
  "the next answer submission will fail once",
  async ({ page }) => {
    let failed = false;
    await page.route(/BatchSubmitAnswers/, async (route) => {
      if (!failed) {
        failed = true;
        await route.abort("failed");
        return;
      }
      await route.continue();
    });
  },
);

When("I retry grading", async ({ page }) => {
  await page.getByRole("button", { name: /^retry grading$/i }).first().click();
});

// QuizResultCard renders "Mark as Correct"/"Mark as Incorrect" depending on
// the card's current state. The standard quiz's mock grader marks every
// non-empty answer correct, so the first such button is always "Mark as
// Incorrect" right after submitting valid answers.
When("I override the first answer", async ({ page }) => {
  await page
    .getByRole("button", { name: /^Mark as (Correct|Incorrect)$/ })
    .first()
    .click();
});

// QuizResultCard renders an "Exclude" outline button next to the override
// toggle. Clicking it marks the result as skipped on the summary view.
When("I exclude the first answer", async ({ page }) => {
  await page.getByRole("button", { name: /^exclude$/i }).first().click();
});

// Between non-final cards the quiz pages auto-advance — there's no button to
// click. At a batch boundary or the final card, BatchFeedback shows a
// "Continue" or "See Results" button after async grading finishes. Wait for
// either to appear (up to 5s) before deciding it's a non-final card.
When("I continue to the next card", async ({ page }) => {
  const button = page
    .getByRole("button", { name: /^(continue|see results)$/i })
    .first();
  try {
    await button.waitFor({ state: "visible", timeout: 5000 });
    await button.click();
  } catch {
    // No BatchFeedback button appeared within 5s — non-final card auto-advanced.
  }
});

// "Finish the quiz" should navigate to /quiz/complete. After the final
// freeform submission, FeedbackActions renders both a "Next Origin"/"Next
// Word" button (primary) and a "See Results" button (outline). We always
// want the latter.
When("I finish the quiz", async ({ page }) => {
  await page.getByRole("button", { name: /^see results$/i }).first().click();
});

Then("I see the quiz mode {string}", async ({ page }, mode: string) => {
  await expect(page.getByText(mode, { exact: true }).first()).toBeVisible();
});

Then("I see the card {string}", async ({ page }, entry: string) => {
  // The active card heading is rendered as <Heading size="xl">{card.entry}</Heading>.
  await expect(page.getByRole("heading", { name: entry }).first()).toBeVisible();
});

Then("I see a meaning prompt", async ({ page }) => {
  // Reverse vocabulary shows the meaning in a Heading and asks for the word.
  // The "Word" hint above the input is unique to this view.
  await expect(page.getByPlaceholder(/type the word/i)).toBeVisible();
});

Then("I see a freeform answer form", async ({ page }) => {
  await expect(page.getByPlaceholder(/e\.g\., hit the hay/i)).toBeVisible();
  await expect(page.getByPlaceholder(/e\.g\., to go to bed/i)).toBeVisible();
});

Then("I see an etymology prompt", async ({ page }) => {
  // Etymology standard placeholder: "type the meaning..."
  // Etymology reverse placeholder:  "type the origin..."
  // Etymology freeform: separate Origin + Meaning inputs (placeholders
  // "e.g., spect" / "e.g., to look or see").
  const anyPrompt = page
    .getByPlaceholder(/type the meaning\.\.\./i)
    .or(page.getByPlaceholder(/type the origin\.\.\./i))
    .or(page.getByPlaceholder(/e\.g\., spect/i));
  await expect(anyPrompt.first()).toBeVisible();
});

Then(
  "the summary shows {int} total words",
  async ({ page }, count: number) => {
    await expect(
      page.getByText(new RegExp(`Total:\\s*${count}\\s*words?`)),
    ).toBeVisible();
  },
);
