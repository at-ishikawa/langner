import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, Then } = createBdd();

// covers route: /analytics
Then("I should be on the Analytics page", async ({ page }) => {
  await expect(page).toHaveURL(/\/analytics(\?|$)/);
});

// Routes covered: "/analytics" and "/analytics/[date]" — both are reached
// when this step runs (the Day List linked from the home page, and the Day
// Detail navigated to directly via the Given step below).
Then("I should be on the Analytics Day Detail page", async ({ page }) => {
  await expect(page).toHaveURL(/\/analytics\/\d{4}-\d{2}-\d{2}/);
});

// Used by both pages to assert no backend error was rendered. The frontend
// catches the RPC error and surfaces "Failed to load <thing>: <message>",
// so we assert that text is not visible. This is the assertion that would
// have failed when the only_full_group_by query bug was present.
Then("the Analytics page is not in an error state", async ({ page }) => {
  // /analytics is the Overview (trends); the Day List lives at /analytics/days.
  // Either surface reports failure as "Failed to load <thing>:", so guard both.
  await expect(page.getByText(/Failed to load (analytics|trends):/)).toHaveCount(0);
});

Then("the Day Detail page is not in an error state", async ({ page }) => {
  // Wait briefly for the API call to settle so the negative assertion is meaningful.
  await page.waitForLoadState("networkidle");
  await expect(page.getByText("Failed to load day:")).toHaveCount(0);
});

// The Day Detail page uses /analytics/{date}. Seed fixtures put misunderstood
// records on 2025-01-02, which lies far outside the default 30-day range, so
// we ask for "all time" via the range query parameter.
Given(
  "I open the Analytics Day Detail for {string}",
  async ({ page }, date: string) => {
    await page.goto(`/analytics/${date}?range=0`);
  },
);
