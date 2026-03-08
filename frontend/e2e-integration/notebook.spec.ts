import { test, expect } from "@playwright/test";

test("navigates to notebooks list and sees notebooks", async ({ page }) => {
  await page.goto("/");

  // Click "Notebooks" feature card on home page
  await page.getByRole("link", { name: /notebooks/i }).click();
  await page.waitForURL("/notebooks");

  // Should see notebooks from example data
  await expect(page.getByText("Friends")).toBeVisible();
  await expect(page.getByText("Vocabulary Examples")).toBeVisible();
});

test("views Friends notebook detail with stories, scenes, and definitions", async ({
  page,
}) => {
  await page.goto("/notebooks");

  // Click on a notebook
  await page.getByRole("link", { name: /Friends/i }).click();
  await page.waitForURL(/\/notebooks\/.+/);

  // Should show notebook header with exact name
  await expect(
    page.getByRole("heading", { name: "Friends", exact: true }),
  ).toBeVisible();

  // Should show word count
  await expect(page.getByText(/\d+ words/)).toBeVisible();

  // Should show story event
  await expect(page.getByText(/Friends S01E01/)).toBeVisible();

  // Should show scene titles
  await expect(
    page.getByText("Central Perk - Morning Coffee"),
  ).toBeVisible();

  // Should show conversations with speakers
  await expect(page.getByText(/Monica/).first()).toBeVisible();

  // Should show definitions
  await expect(page.getByText("break the ice").first()).toBeVisible();

  // Should show learning status badges (Badge components, not select options)
  const badges = page.locator(".chakra-badge");
  await expect(badges.first()).toBeVisible();

  // Verify various statuses are present in badges
  await expect(badges.filter({ hasText: "Learning" }).first()).toBeVisible();
  await expect(
    badges.filter({ hasText: "Misunderstood" }).first(),
  ).toBeVisible();
  await expect(
    badges.filter({ hasText: "Understood" }).first(),
  ).toBeVisible();
  await expect(badges.filter({ hasText: "Usable" }).first()).toBeVisible();
  await expect(
    badges.filter({ hasText: "Intuitive" }).first(),
  ).toBeVisible();
});

test("filters definitions by learning status", async ({ page }) => {
  await page.goto("/notebooks");
  await page.getByRole("link", { name: /Friends/i }).click();
  await page.waitForURL(/\/notebooks\/.+/);

  // Filter by "Misunderstood"
  await page.locator("select").selectOption("misunderstood");

  // Should show misunderstood words in badges
  const badges = page.locator(".chakra-badge");
  await expect(
    badges.filter({ hasText: "Misunderstood" }).first(),
  ).toBeVisible();

  // Words that are only "Intuitive" should be hidden
  await expect(page.getByText("look forward to")).not.toBeVisible();
});

test("views Vocabulary Examples notebook with flashcard words", async ({
  page,
}) => {
  await page.goto("/notebooks");

  await page.getByRole("link", { name: /Vocabulary Examples/i }).click();
  await page.waitForURL(/\/notebooks\/.+/);

  // Should show notebook header
  await expect(
    page.getByRole("heading", { name: "Vocabulary Examples", exact: true }),
  ).toBeVisible();

  // Should show word count
  await expect(page.getByText(/\d+ words/)).toBeVisible();

  // Should show vocabulary words from example data
  await expect(page.getByText("serendipity").first()).toBeVisible();
  await expect(page.getByText("ephemeral").first()).toBeVisible();
  await expect(page.getByText("ubiquitous").first()).toBeVisible();
});

test("expands Vocabulary Examples word card to show details and learning history", async ({
  page,
}) => {
  await page.goto("/notebooks");
  await page.getByRole("link", { name: /Vocabulary Examples/i }).click();
  await page.waitForURL(/\/notebooks\/.+/);

  await expect(page.getByText("serendipity").first()).toBeVisible();

  // Click on the word card to expand it
  await page.getByText("serendipity").first().click();

  // Should show expanded details
  await expect(page.getByText("Part of speech:")).toBeVisible();
  await expect(page.getByText("noun")).toBeVisible();
  await expect(page.getByText("Pronunciation:")).toBeVisible();

  // Should show learning history
  await expect(page.getByText("Learning History:")).toBeVisible();
  await expect(page.getByText("2026-02-20")).toBeVisible();
});

test("Vocabulary Examples shows correct learning status badges", async ({
  page,
}) => {
  await page.goto("/notebooks");
  await page.getByRole("link", { name: /Vocabulary Examples/i }).click();
  await page.waitForURL(/\/notebooks\/.+/);

  const badges = page.locator(".chakra-badge");
  await expect(badges.first()).toBeVisible();

  // serendipity is usable, ephemeral is misunderstood, ameliorate is intuitive
  await expect(badges.filter({ hasText: "Usable" }).first()).toBeVisible();
  await expect(badges.filter({ hasText: "Misunderstood" }).first()).toBeVisible();
  await expect(badges.filter({ hasText: "Intuitive" }).first()).toBeVisible();
});
