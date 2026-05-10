import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, When, Then } = createBdd();

const NOTEBOOK_IDS: Record<string, string> = {
  Idioms: "idioms",
  "Word Roots": "word-roots",
};

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

When("I follow the {string} link", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
});

When("I open the {string} notebook", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
});

When("I switch to the {string} tab", async ({ page }, name: string) => {
  await page.getByText(name, { exact: true }).first().click();
});

When("I open the {string} word card", async ({ page }, entry: string) => {
  await page.getByText(entry).first().click();
});

// covers route: /notebooks/etymology/[id]/mindmap
When("I open the mindmap for {string}", async ({ page }, origin: string) => {
  await page
    .getByRole("link", { name: new RegExp(`mindmap`, "i") })
    .first()
    .click();
});

When("I choose the {string} quiz mode", async ({ page }, mode: string) => {
  await page.getByText(mode, { exact: true }).first().click();
});

When("I select the {string} notebook", async ({ page }, name: string) => {
  await page.getByRole("checkbox", { name: new RegExp(name) }).click({ force: true });
});

// Starting a quiz navigates to one of the per-mode pages:
//   /quiz/standard, /quiz/reverse, /quiz/freeform,
//   /quiz/etymology-standard, /quiz/etymology-reverse, /quiz/etymology-freeform
When("I start the quiz", async ({ page }) => {
  await page.getByRole("button", { name: /^start$/i }).click();
});

When("I type the answer {string}", async ({ page }, answer: string) => {
  await page.getByPlaceholder(/answer/i).first().fill(answer);
});

When("I type the word {string}", async ({ page }, word: string) => {
  await page.getByLabel(/word/i).first().fill(word);
});

When("I type the meaning {string}", async ({ page }, meaning: string) => {
  await page.getByLabel(/meaning/i).first().fill(meaning);
});

When("I submit my answer", async ({ page }) => {
  const submit = page.getByRole("button", { name: /(submit|enter|next)/i }).first();
  if (await submit.isVisible().catch(() => false)) {
    await submit.click();
  } else {
    await page.keyboard.press("Enter");
  }
});

When("I continue to the next card", async ({ page }) => {
  await page
    .getByRole("button", { name: /(continue|next|see results)/i })
    .first()
    .click();
});

When("I finish the quiz", async ({ page }) => {
  await page
    .getByRole("button", { name: /(finish|see results|continue|done)/i })
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
  await expect(page.getByText(entry).first()).toBeVisible();
});

Then("I see the example {string}", async ({ page }, example: string) => {
  await expect(page.getByText(example).first()).toBeVisible();
});

Then("I see the origin {string}", async ({ page }, origin: string) => {
  await expect(page.getByText(origin).first()).toBeVisible();
});

Then("I see the node {string}", async ({ page }, label: string) => {
  await expect(page.getByText(label).first()).toBeVisible();
});

Then("I see the card {string}", async ({ page }, entry: string) => {
  await expect(page.getByRole("heading", { name: entry }).first()).toBeVisible();
});

Then("I see a meaning prompt", async ({ page }) => {
  await expect(page.getByText(/meaning/i).first()).toBeVisible();
});

Then("I see a freeform answer form", async ({ page }) => {
  await expect(page.getByLabel(/word/i).first()).toBeVisible();
  await expect(page.getByLabel(/meaning/i).first()).toBeVisible();
});

Then("I see an etymology prompt", async ({ page }) => {
  await expect(page.getByPlaceholder(/answer/i).first()).toBeVisible();
});

Then(
  "the summary shows {int} total words",
  async ({ page }, count: number) => {
    await expect(page.getByText(new RegExp(`Total:\\s*${count}\\s*words?`))).toBeVisible();
  },
);
