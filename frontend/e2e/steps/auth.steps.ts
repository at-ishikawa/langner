import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, Then } = createBdd();

// covers route: /login — the Google sign-in entry point. The SessionProvider
// guard only redirects *away* from other routes to /login; it never redirects
// away from /login itself, so this renders under the injected session cookie.
Given("I am on the login page", async ({ page }) => {
  await page.goto("/login");
});

Then("I see the {string} button", async ({ page }, name: string) => {
  await expect(page.getByRole("button", { name })).toBeVisible();
});
