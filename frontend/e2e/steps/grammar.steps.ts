import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// Opens the Grammar tab of the Quiz hub. Mirrors "I open the Relearn Quiz":
// the tab is a clickable <Text>, and GrammarStart renders inline underneath.
When("I open the Grammar quiz", async ({ page }) => {
  await page.goto("/quiz");
  await page.getByText("Grammar", { exact: true }).click();
  await expect(page.getByText(/Fix the grammar mistakes/i)).toBeVisible();
});

// GrammarStart's "Start" button navigates to the session page.
When("I start the grammar quiz", async ({ page }) => {
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page).toHaveURL(/\/quiz\/grammar(\?|$)/);
});

// Each blank renders an <Input aria-label={`Correction for "<span>"`}> beside
// its struck-through wrong span.
Then(
  "I see the grammar correction input for {string}",
  async ({ page }, incorrect: string) => {
    await expect(page.getByLabel(`Correction for "${incorrect}"`)).toBeVisible();
  },
);

// Each box is graded the moment it's committed (Enter), so filling + Enter
// grades that one blank in place — there is no batch "check" step.
When(
  "I correct {string} with {string}",
  async ({ page }, incorrect: string, answer: string) => {
    const box = page.getByLabel(`Correction for "${incorrect}"`);
    await box.fill(answer);
    await box.press("Enter");
  },
);

// After grading, each blank becomes a pill whose accessible name carries its
// status, e.g. `the John — correct`.
Then(
  "the graded post marks {string} as correct",
  async ({ page }, incorrect: string) => {
    await expect(
      page.getByRole("button", { name: `${incorrect} — correct` }),
    ).toBeVisible();
  },
);

Then(
  "the graded post marks {string} as incorrect",
  async ({ page }, incorrect: string) => {
    await expect(
      page.getByRole("button", { name: `${incorrect} — incorrect` }),
    ).toBeVisible();
  },
);

// The single-post session ends on "See results"; a multi-post one would show
// "Next post". Accept either, then assert we reached the summary.
When("I finish the grammar post", async ({ page }) => {
  await page
    .getByRole("button", { name: /^(see results|next post)$/i })
    .first()
    .click();
  await expect(page).toHaveURL(/\/quiz\/grammar\/complete/);
});

Then("I should be on the Grammar Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/grammar\/complete/);
  await expect(
    page.getByRole("heading", { name: "Grammar Session Complete" }),
  ).toBeVisible();
});

// QuizResultCard appends "(overridden)" once Mark-as-Correct succeeds — proof
// the OverrideAnswer RPC round-tripped for a grammar note.
Then("the summary shows an overridden correction", async ({ page }) => {
  await expect(page.getByText("(overridden)").first()).toBeVisible();
});

// The grouped list moves excluded results under an "Excluded from Quizzes"
// heading once SkipWord succeeds.
Then("the summary shows an excluded correction", async ({ page }) => {
  await expect(page.getByText(/Excluded from Quizzes/i)).toBeVisible();
});
