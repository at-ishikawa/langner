import { expect, test, type Locator, type Page } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, When, Then, BeforeScenario } = createBdd();

// Layer 3 coverage relies on TestStep.params?.url, which Playwright only
// populates for explicit page.goto calls. Click-driven router.push navigations
// (e.g. /quiz/standard after pressing "Start") never show up there.
//
// We hook into the page's framenavigated event and attach each URL as a test
// info annotation so coverage-reporter.ts can read them in onTestEnd.
BeforeScenario(async ({ page }) => {
  const info = test.info();
  page.on("framenavigated", (frame) => {
    if (frame.parentFrame() !== null) return; // only main frame
    const url = frame.url();
    if (!url || url === "about:blank") return;
    info.annotations.push({ type: "visited-url", description: url });
  });
});

const NOTEBOOK_IDS: Record<string, string> = {
  Idioms: "idioms",
  "Word Roots": "word-roots",
  "Short Tales": "short-tales",
};

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

Given("I am on the home page", async ({ page }) => {
  await page.goto("/");
});

Given("I am on the Learn page", async ({ page }) => {
  await page.goto("/learn");
});

Given("I am on the Quiz page", async ({ page }) => {
  await page.goto("/quiz");
});

Given("I am on the {string} notebook detail page", async ({ page }, name: string) => {
  const id = NOTEBOOK_IDS[name];
  if (!id) throw new Error(`Unknown notebook: ${name}`);
  await page.goto(`/notebooks/${id}`);
});

// covers route: /notebooks/etymology/[id]
Given(
  "I am on the {string} etymology notebook page",
  async ({ page }, name: string) => {
    const id = NOTEBOOK_IDS[name];
    if (!id) throw new Error(`Unknown notebook: ${name}`);
    await page.goto(`/notebooks/etymology/${id}`);
  },
);

// covers route: /learn/[id]
Given(
  "I am on the {string} learn content page",
  async ({ page }, name: string) => {
    const id = NOTEBOOK_IDS[name];
    if (!id) throw new Error(`Unknown notebook: ${name}`);
    await page.goto(`/learn/${id}`);
  },
);

When("I follow the {string} link", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
});

When("I open the {string} notebook", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
});

When("I switch to the {string} tab", async ({ page }, name: string) => {
  // Tab labels are <Text> inside a clickable <Box>. There is exactly one
  // visible match per tab (Vocabulary / Etymology / All Origins / By Meaning).
  await page.getByText(name, { exact: true }).first().click();
});

// On /notebooks/[id] (flashcard or story-list view) each story is a clickable
// card. After clicking, the user lands on the in-page story detail.
When("I open the {string} story", async ({ page }, name: string) => {
  await page.getByText(name, { exact: true }).first().click();
});

// On the story detail view of /notebooks/[id], each WordCard renders the
// expression in a <Text fontWeight="semibold">. The card itself is a
// clickable Box, so clicking the expression text expands the card.
When("I open the {string} word card", async ({ page }, entry: string) => {
  await page.getByText(entry, { exact: true }).first().click();
});

// covers route: /notebooks/etymology/[id]/mindmap
// On the etymology notebook detail page, the user first opens an origin card
// (which navigates to ?origin=<name>), then clicks the "View Mindmap" link.
When("I open the mindmap for {string}", async ({ page }, origin: string) => {
  // Click the origin card by its visible origin text.
  await page.getByText(origin, { exact: true }).first().click();
  // The "View Mindmap" link only renders after an origin is selected.
  await page.getByRole("link", { name: /view mindmap/i }).first().click();
});

When("I choose the {string} quiz mode", async ({ page }, mode: string) => {
  // Each mode card is a clickable Box. The mode title is the first <Text>;
  // the description below it doesn't contain "Standard"/"Reverse"/"Freeform"
  // as a standalone word, so exact-match on the mode name is unique.
  await page.getByText(mode, { exact: true }).first().click();
});

When("I include unstudied words", async ({ page }) => {
  await switchLabel(page, "Include unstudied words").click();
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

// Between non-final cards the quiz pages auto-advance — there's no button to
// click. At a batch boundary or the final card, BatchFeedback shows a
// "Continue" or "See Results" button. We click whichever is visible, and
// otherwise treat the step as a no-op so the feature reads naturally.
When("I continue to the next card", async ({ page }) => {
  const button = page
    .getByRole("button", { name: /^(continue|see results)$/i })
    .first();
  if (await button.isVisible().catch(() => false)) {
    await button.click();
  }
});

// "Finish the quiz" should navigate to /quiz/complete. After the final
// freeform submission, FeedbackActions renders both a "Next Origin"/"Next
// Word" button (primary) and a "See Results" button (outline). We always
// want the latter.
When("I finish the quiz", async ({ page }) => {
  await page
    .getByRole("button", { name: /^see results$/i })
    .first()
    .click();
});

Then("I should be on the Learn page", async ({ page }) => {
  await expect(page).toHaveURL(/\/learn(\/|$)/);
});

Then("I should be on the Quiz page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz(\/?|$)/);
});

// covers route: /quiz/complete
Then("I should be on the Quiz Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/complete/);
});

Then("I should be on the mindmap page", async ({ page }) => {
  await expect(page).toHaveURL(/\/mindmap/);
});

Then("I see the notebook {string}", async ({ page }, name: string) => {
  await expect(page.getByText(name).first()).toBeVisible();
});

Then("I see the quiz mode {string}", async ({ page }, mode: string) => {
  await expect(page.getByText(mode, { exact: true }).first()).toBeVisible();
});

Then("I see the heading {string}", async ({ page }, name: string) => {
  await expect(page.getByRole("heading", { name }).first()).toBeVisible();
});

Then("I see the word {string}", async ({ page }, entry: string) => {
  await expect(page.getByText(entry, { exact: true }).first()).toBeVisible();
});

Then("I see the example {string}", async ({ page }, example: string) => {
  await expect(page.getByText(example).first()).toBeVisible();
});

Then("I see the origin {string}", async ({ page }, origin: string) => {
  await expect(page.getByText(origin, { exact: true }).first()).toBeVisible();
});

// The mindmap renders nodes as <div> with text inside (ReactFlow). The
// focused-origin node label is "<origin>\n(<meaning>)". Match a substring
// of that text so the test doesn't depend on exact whitespace handling.
Then("I see the node {string}", async ({ page }, label: string) => {
  await expect(
    page.getByText(new RegExp(label, "i")).first(),
  ).toBeVisible();
});

Then("I see the card {string}", async ({ page }, entry: string) => {
  // The active card heading is rendered as <Heading size="xl">{card.entry}</Heading>.
  await expect(
    page.getByRole("heading", { name: entry }).first(),
  ).toBeVisible();
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

// covers route: /learn/[id]
Then("I should be on the Learn content page", async ({ page }) => {
  await expect(page).toHaveURL(/\/learn\/[A-Za-z0-9_-]+/);
});
