import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

test("Dashboard motion off makes the home view still", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" }); // the config defaults to reduce
  await stubDash(page, { live: [{ ...S1, state: "permission" }] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-home")).not.toHaveClass(/still/);
  await page.getByTestId("open-settings").click();
  await page.getByTestId("dash-motion-toggle").uncheck();
  await expect(page.getByTestId("dash-home")).toHaveClass(/still/);
  const anim = await page.locator('[data-testid="dash-tile"]').first().evaluate((el) => getComputedStyle(el).animationName);
  expect(anim).toBe("none");
});

test("reduced motion makes the home view still", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-home")).toHaveClass(/still/);
});
