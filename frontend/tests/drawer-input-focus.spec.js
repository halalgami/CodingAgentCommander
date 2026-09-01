import { test, expect } from "@playwright/test";

// Guards a regression that the whole suite missed: a drawer that reclaims focus
// on focusout steals it from an input mid-click, because a real mouse click
// blurs the old element BEFORE focusing the new one. Every drawer input became
// untypable while 99 specs stayed green — fill() sets focus without blurring
// first, so nothing exercised the real sequence.
//
// Use click() + keyboard.type() here, never fill(), or this stops testing the
// thing it exists to test.
async function stubProviders(page) {
  await page.addInitScript(() => {
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ListProviders: async () => [
        { type: "ollama-cloud", defined: true, active: false, apiBase: "https://ollama.com", region: "", modelCnt: 0 },
      ],
      KeyStatus: async () => [{ env: "OLLAMA_API_KEY", set: false, optional: false }],
      ListSessions: async () => [],
      ListProjects: async () => [],
      Config: async () => [],
      Models: async () => [],
    });
  });
}

test("a drawer input keeps focus through a real mouse click and accepts typing", async ({ page }) => {
  await stubProviders(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-providers").click();

  const input = page.getByTestId("key-input-OLLAMA_API_KEY");
  await expect(input).toBeVisible();
  await input.click();
  await expect(input).toBeFocused();
  await page.keyboard.type("sk-typed-by-hand");
  await expect(input).toHaveValue("sk-typed-by-hand");
});

test("Escape still closes a drawer after focus was lost to the body", async ({ page }) => {
  await stubProviders(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();

  // Park focus on <body>, the state a destroyed focused element leaves behind.
  await page.evaluate(() => document.activeElement?.blur());
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("drawer-providers")).not.toBeVisible();
});
