import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, When, Then } = createBdd();

const NOTEBOOK_IDS: Record<string, string> = {
  Idioms: "idioms",
  "Word Roots": "word-roots",
  "Short Tales": "short-tales",
};

Given(
  "I am on the {string} notebook detail page",
  async ({ page }, name: string) => {
    const id = NOTEBOOK_IDS[name];
    if (!id) throw new Error(`Unknown notebook: ${name}`);
    await page.goto(`/notebooks/${id}`);
  },
);

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

When("I open the {string} notebook", async ({ page }, name: string) => {
  await page.getByRole("link", { name: new RegExp(name, "i") }).first().click();
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
  await page.getByText(origin, { exact: true }).first().click();
  await page.getByRole("link", { name: /view mindmap/i }).first().click();
});

Then("I see the origin {string}", async ({ page }, origin: string) => {
  await expect(page.getByText(origin, { exact: true }).first()).toBeVisible();
});

// The mindmap renders nodes as <div> with text inside (ReactFlow). The
// focused-origin node label is "<origin>\n(<meaning>)". Match a substring
// of that text so the test doesn't depend on exact whitespace handling.
Then("I see the node {string}", async ({ page }, label: string) => {
  await expect(page.getByText(new RegExp(label, "i")).first()).toBeVisible();
});
