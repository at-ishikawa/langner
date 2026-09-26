import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, Then } = createBdd();

// covers route: /login — the Google sign-in entry point. The SessionProvider
// guard treats /login as public: it never silently re-authenticates away from
// /login, so this renders even under the injected access token.
Given("I am on the login page", async ({ page }) => {
  await page.goto("/login");
});

Then("I see the {string} button", async ({ page }, name: string) => {
  await expect(page.getByRole("button", { name })).toBeVisible();
});
