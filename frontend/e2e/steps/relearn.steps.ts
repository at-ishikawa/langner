import { expect } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { When, Then } = createBdd();

// covers route: /quiz/relearn
When("I open the Relearn Quiz", async ({ page }) => {
  await page.goto("/quiz");
  await page.getByRole("link", { name: /Relearn recent mistakes/i }).click();
  await expect(page).toHaveURL(/\/quiz\/relearn$/);
});

Then("I see words to relearn", async ({ page }) => {
  await expect(page.getByText(/to relearn/i)).toBeVisible();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeEnabled();
});

// covers route: /quiz/relearn/session
When("I start the relearn session", async ({ page }) => {
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page).toHaveURL(/\/quiz\/relearn\/session/);
});

Then("I see a relearn card", async ({ page }) => {
  await expect(page.getByText(/words? left/i)).toBeVisible();
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeVisible();
});

// Loops through the working queue until it empties and the session lands on the
// complete page. The pool can hold both vocabulary and etymology-origin words,
// and the mock etymology grader only accepts the exact meaning. So the loop
// captures each card's meaning from its feedback and reuses it on the next
// encounter: an etymology card is answered wrong once (recording its meaning),
// then correct. Vocabulary cards clear on the first non-"wrong" answer. Every
// card therefore clears within two passes, so the loop converges.
//
// exact:true is required on the button names: `next dev` injects a "Open
// Next.js Dev Tools" button whose accessible name contains "Next", which a
// loose name match collides with.
When("I clear every remaining relearn card", async ({ page }) => {
  const meanings = new Map<string, string>();
  const submit = page.getByRole("button", { name: "Submit", exact: true });
  const next = page.getByRole("button", { name: "Next", exact: true });
  for (let i = 0; i < 200 && !page.url().includes("/quiz/relearn/complete"); i++) {
    await submit.waitFor({ state: "visible" });
    const entry = ((await page.getByTestId("relearn-entry").textContent()) ?? "").trim();
    await page.getByPlaceholder("Type the meaning").fill(meanings.get(entry) ?? "an attempt");
    await submit.click();
    const meaningEl = page.getByTestId("relearn-meaning");
    await meaningEl.waitFor({ state: "visible" });
    meanings.set(entry, ((await meaningEl.textContent()) ?? "").trim());
    await next.click();
    // Wait until either the next card's input mounts or the session navigates
    // to the complete page — avoids racing the blank transition frame.
    await page.waitForFunction(
      () =>
        location.pathname.includes("/quiz/relearn/complete") ||
        !!document.querySelector('input[placeholder="Type the meaning"]'),
      undefined,
      { timeout: 15000 },
    );
  }
});

// covers route: /quiz/relearn/complete
Then("I should be on the Relearn Complete page", async ({ page }) => {
  await expect(page).toHaveURL(/\/quiz\/relearn\/complete/);
  await expect(page.getByRole("heading", { name: "Relearn complete" })).toBeVisible();
});
