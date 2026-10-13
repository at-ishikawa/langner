import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// The Settings page renders one row per LLM provider: an API-key input
// (aria-label "<provider> API key"), a "Save <provider> key" button, a
// "Make <provider> primary" button once a key is set, and a "Remove <provider>"
// button. Keys are write-only, so these steps assert the state copy, never the
// key value.

When("I enter the {string} API key {string}", async ({ page }, provider: string, key: string) => {
  await page.getByLabel(`${provider} API key`).fill(key);
});

When("I save the {string} key", async ({ page }, provider: string) => {
  await page.getByRole("button", { name: `Save ${provider} key` }).click();
});

When("I make {string} the primary provider", async ({ page }, provider: string) => {
  await page.getByRole("button", { name: `Make ${provider} primary` }).click();
});

When("I remove the {string} key", async ({ page }, provider: string) => {
  await page.getByRole("button", { name: new RegExp(`^Remove ${provider}$`, "i") }).click();
});

// Generic visible-text assertion for the Settings status copy ("No API key
// configured yet.", "No key set.", "Saved."). Substring match.
Then("I see the text {string}", async ({ page }, text: string) => {
  await expect(page.getByText(text, { exact: false }).first()).toBeVisible();
});
