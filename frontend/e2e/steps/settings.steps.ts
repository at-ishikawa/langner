import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// The Settings page renders the per-user LLM key form: a provider <select>
// (defaults to the first available provider), a password API-key input
// (placeholder "sk-…"), a Save button, and — once a key is set — a Remove
// button. The key is write-only, so these steps assert the state text, never
// the key value.

When("I enter the API key {string}", async ({ page }, key: string) => {
  await page.locator('input[type="password"]').fill(key);
});

When("I save the API key", async ({ page }) => {
  await page.getByRole("button", { name: /^save$/i }).click();
});

When("I remove the API key", async ({ page }) => {
  await page.getByRole("button", { name: /^remove key$/i }).click();
});

// Generic visible-text assertion for the Settings state copy ("No API key
// configured yet.", "API key saved.", "… — key set."). Substring match so the
// "key set." fragment within the longer "Current: <provider> … — key set."
// line still matches.
Then("I see the text {string}", async ({ page }, text: string) => {
  await expect(page.getByText(text, { exact: false }).first()).toBeVisible();
});
