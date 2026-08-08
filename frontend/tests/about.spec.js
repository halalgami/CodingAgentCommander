import { test, expect } from "@playwright/test";

// Stub the two bindings the About modal uses (plain browser has neither).
async function stubAbout(page) {
  await page.addInitScript(() => {
    window.go = window.go || {}; window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      GetBuildInfo: async () => ({ version: "9.9.9", commit: "abc1234", buildDate: "2026-08-08" }),
    });
    window.runtime = window.runtime || {};
    window.runtime.BrowserOpenURL = (u) => { window.__opened = u; };
  });
}

test("About opens from the footer with identity, version and tagline", async ({ page }) => {
  await stubAbout(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-about").click();
  const modal = page.getByTestId("about-modal");
  await expect(modal).toBeVisible();
  await expect(modal).toContainText("The terminal is the workspace");
  await expect(modal).toContainText("Algam");
  await expect(modal).toContainText("MIT");
  await expect(page.getByTestId("about-version")).toContainText("9.9.9");
});

test("About website link opens algamthe.dev via BrowserOpenURL", async ({ page }) => {
  await stubAbout(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-about").click();
  await page.getByTestId("about-link").click();
  const opened = await page.evaluate(() => window.__opened);
  expect(opened).toBe("https://algamthe.dev");
});

test("About closes on Escape", async ({ page }) => {
  await stubAbout(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-about").click();
  await expect(page.getByTestId("about-modal")).toBeVisible();
  await page.getByTestId("about-modal").press("Escape");
  await expect(page.getByTestId("about-modal")).not.toBeVisible();
});
