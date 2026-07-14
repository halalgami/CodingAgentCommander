import { test, expect } from "@playwright/test";

// Wails bindings (window.go) don't exist in a plain browser; binding calls
// reject but the shell MUST still mount (the B1 blank-window bug class).
test("app mounts the deck shell", async ({ page }) => {
  await page.goto("/?nointro");
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("wordmark")).toHaveText("COMMANDER");
  await expect(page.getByTestId("launch-button")).toBeVisible();
  await expect(page.getByTestId("folder-input")).toBeVisible();
  await expect(page.getByTestId("empty-state")).toBeVisible();
});

test("providers drawer opens and closes on ESC", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("drawer-providers")).not.toBeVisible();
});

test("models drawer opens with add form", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-models").click();
  await expect(page.getByTestId("drawer-models")).toBeVisible();
  await expect(page.getByTestId("add-model-id")).toBeVisible();
  await expect(page.getByTestId("add-model-submit")).toBeVisible();
});

test("launch without folder shows inline error, not a toast", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("launch-button").click();
  await expect(page.getByTestId("launch-error")).toBeVisible();
});

test("accent choice persists across reload", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("accent-hue").fill("200");
  await page.reload();
  await page.goto("/?nointro");
  const accent = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue("--accent"),
  );
  if (!accent.includes("200")) throw new Error(`accent not persisted: ${accent}`);
});

test("command palette opens on ⌘K and closes on Escape", async ({ page }) => {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  await expect(page.getByTestId("palette-input")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("palette")).not.toBeVisible();
});

test("boot intro suppressed once played flag is set", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => localStorage.setItem("commander.introPlayed.v1", "1"));
  await page.goto("/"); // no ?nointro: flag alone must suppress it
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("boot-intro")).not.toBeVisible();
});

test("palette fuzzy-filters and opens settings drawer on Enter", async ({ page }) => {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await page.getByTestId("palette-input").fill("set");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("drawer-settings")).toBeVisible();
});

test("font size pref persists and hotkey bumps it", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("font-size").fill("16");
  await page.keyboard.press("Escape");
  await page.keyboard.press("Meta+Equal");
  await page.reload();
  await page.goto("/?nointro");
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v1")).fontSize);
  if (stored !== 17) throw new Error(`expected 17, got ${stored}`);
});

test("sidebar divider drag persists width", async ({ page }) => {
  await page.goto("/?nointro");
  const d = page.getByTestId("sidebar-divider");
  const box = await d.boundingBox();
  await page.mouse.move(box.x + 2, box.y + 200);
  await page.mouse.down();
  await page.mouse.move(380, box.y + 200);
  await page.mouse.up();
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v1")).sidebarW);
  if (stored < 360 || stored > 400) throw new Error(`expected ~380, got ${stored}`);
});

test("rc auto toggle persists", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("rc-auto-toggle").check();
  await page.reload();
  await page.goto("/?nointro");
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v1")).rcAutoEnable);
  if (stored !== true) throw new Error("rcAutoEnable not persisted");
});
