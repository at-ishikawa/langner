import { expect, test } from "@playwright/test";
import { createBdd } from "playwright-bdd";
import { resetState } from "../support/reset";

const { Given, When, Then, BeforeScenario } = createBdd();

// Restore the seeded learning-history baseline (DB + learning_notes YAML)
// before every scenario. The stack shares one seeded DB with no reset between
// scenarios, and DB-mode reads now reflect earlier scenarios' quiz writes, so
// without this a word answered in an earlier scenario is no longer due in a
// later one. Each scenario sets up its own words/misses, so a full reset is
// safe. Runs before the scenario's steps navigate, so the first page load
// already sees baseline data.
BeforeScenario(async () => {
  resetState();
});

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

Given("I am on the home page", async ({ page }) => {
  await page.goto("/");
});

Given("I am on the Learn page", async ({ page }) => {
  await page.goto("/learn");
});

Given("I am on the Quiz page", async ({ page }) => {
  await page.goto("/quiz");
});

When("I follow the {string} link", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
});

// Generic button click used by "Back to Start" on /quiz/complete and any other
// page that exposes a single button by visible text.
When("I follow the {string} button", async ({ page }, name: string) => {
  await page.getByRole("button", { name: new RegExp(`^${name}$`, "i") }).first().click();
});

When("I switch to the {string} tab", async ({ page }, name: string) => {
  // Tab labels are <Text> inside a clickable <Box>. There is exactly one
  // visible match per tab (Vocabulary / Etymology / All Origins / By Meaning).
  await page.getByText(name, { exact: true }).first().click();
});

Then("I should be on the Learn page", async ({ page }) => {
  await expect(page).toHaveURL(/\/learn(\/|$)/);
});

Then("I should be on the Quiz page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz(\/?|$)/);
});

// covers route: /history — the bare History list. Anchored so it matches
// /history and /history?<query> but not the /history/[date] Day Detail page.
Then("I should be on the History page", async ({ page }) => {
  await expect(page).toHaveURL(/\/history(\?|$)/);
});

// covers route: /quiz/complete
Then("I should be on the Quiz Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/complete/);
});

Then("I should be on the mindmap page", async ({ page }) => {
  await expect(page).toHaveURL(/\/mindmap/);
});

// covers route: /learn/[id]
Then("I should be on the Learn content page", async ({ page }) => {
  await expect(page).toHaveURL(/\/learn\/[A-Za-z0-9_-]+/);
});

Then("I see the heading {string}", async ({ page }, name: string) => {
  await expect(page.getByRole("heading", { name }).first()).toBeVisible();
});

Then("I see the notebook {string}", async ({ page }, name: string) => {
  await expect(page.getByText(name).first()).toBeVisible();
});

// Negative counterpart: the notebook must be absent from the picker entirely.
// Used to assert the Vocabulary tab hides a notebook with no vocabulary (an
// empty journal, or a grammar-only summary) — including the Journal+Grammar
// duplicate id, which collapses to nothing when the journal has no words.
Then("I do not see the notebook {string}", async ({ page }, name: string) => {
  await expect(page.getByText(name)).toHaveCount(0);
});

Then("I see the word {string}", async ({ page }, entry: string) => {
  await expect(page.getByText(entry, { exact: true }).first()).toBeVisible();
});

Then("I see the example {string}", async ({ page }, example: string) => {
  await expect(page.getByText(example).first()).toBeVisible();
});
