import { test, expect } from "@playwright/test";

// Stubs enough of the Wails surface for LaunchPanel to render its picker.
async function stubModels(page, models) {
  await page.addInitScript((seed) => {
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      Config: async () => seed,
      Models: async () => seed,
      ListProviders: async () => [],
      KeyStatus: async () => [],
      ListSessions: async () => [],
      ListProjects: async () => [],
    });
  }, models);
}

const FLAT = [
  { id: "claude-opus-5-5", label: "Anthropic · Opus 5.5", provider: "anthropic", routed: false, ready: true, default: true },
  { id: "claude-opus-5", label: "Anthropic · Opus 5", provider: "anthropic", routed: false, ready: true, default: false },
  { id: "ollama-glm", label: "Ollama · glm-5.3", provider: "ollama-cloud", routed: true, ready: true, default: false },
  { id: "ollama-qwen", label: "Ollama · qwen", provider: "ollama-cloud", routed: true, ready: false, default: false },
];

async function openPicker(page) {
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").click();
  return sel;
}

test("the picker lists every model", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await expect(sel.locator('li[role="option"]')).toHaveCount(4);
});

test("a routed model with no key is marked", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await expect(sel.getByRole("option", { name: /qwen/ })).toContainText("(needs key)");
});

test("clicking an option selects it and closes the list", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await sel.getByRole("option", { name: /glm-5\.3/ }).locator("button").click();
  await expect(sel.locator('ul[role="listbox"]')).toHaveCount(0);
  await expect(sel.locator("button.trigger")).toContainText("glm-5.3");
});

// Real key events, not fill(): this repo shipped a focus bug past 99 green
// specs because fill() focuses without blurring. Keyboard paths get driven.
test("keyboard opens, moves and selects", async ({ page }) => {
  await stubModels(page, FLAT);
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").focus();
  await page.keyboard.press("ArrowDown");          // opens, highlights index 0
  await expect(sel.locator('ul[role="listbox"]')).toBeVisible();
  await page.keyboard.press("ArrowDown");          // index 1
  await page.keyboard.press("Enter");
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\s*▾?\s*$/);
});

test("Escape closes without selecting", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await page.keyboard.press("ArrowDown");          // move highlight to index 1
  await page.keyboard.press("Escape");             // close without committing
  await expect(sel.locator('ul[role="listbox"]')).toHaveCount(0);
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\.5\s*▾?\s*$/);
});

test("models are grouped under provider headers, anthropic first", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  const headers = sel.locator('li[role="presentation"]');
  await expect(headers).toHaveCount(2);
  await expect(headers.nth(0)).toContainText("Anthropic");
  await expect(headers.nth(0)).toContainText("2");
  await expect(headers.nth(1)).toContainText("Ollama Cloud");
  await expect(headers.nth(1)).toContainText("2");
});

test("the provider prefix is dropped from rows under a header", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await expect(sel.getByRole("option", { name: "Opus 5.5" })).toBeVisible();
  await expect(sel.locator('li[role="option"]').first()).not.toContainText("Anthropic ·");
});

test("collapsing a group hides its rows but keeps the count, and survives reload", async ({ page }) => {
  await stubModels(page, FLAT);
  let sel = await openPicker(page);
  await sel.locator('li[role="presentation"]', { hasText: "Ollama Cloud" }).locator("button").click();
  await expect(sel.getByRole("option", { name: /glm/ })).toHaveCount(0);
  await expect(sel.locator('li[role="presentation"]', { hasText: "Ollama Cloud" })).toContainText("2");

  await page.reload();
  sel = await openPicker(page);
  await expect(sel.getByRole("option", { name: /glm/ })).toHaveCount(0);
});

// User decision 2026-10-04: every group collapses, including the one holding the
// selection. The closed trigger already names the selection, so the list need not.
test("the group holding the selection can be collapsed and the trigger keeps its full label", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  const hdr = sel.locator('li[role="presentation"]', { hasText: "Anthropic" }).locator("button");
  await expect(hdr).toBeEnabled();
  await expect(hdr).toHaveAttribute("aria-expanded", "true");
  const caret = hdr.locator("[data-state]");
  await expect(caret).toHaveAttribute("data-state", "expanded");
  await hdr.click();
  await expect(hdr).toHaveAttribute("aria-expanded", "false");
  await expect(caret).toHaveAttribute("data-state", "collapsed");
  await hdr.click();
  await expect(caret).toHaveAttribute("data-state", "expanded");
  await hdr.click();
  await expect(sel.getByRole("option", { name: "Opus 5.5" })).toHaveCount(0);
  await expect(sel.getByRole("option", { name: "Opus 5", exact: true })).toHaveCount(0);
  await expect(hdr.locator(".count")).toHaveText("2");
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v2")));
  expect(stored.collapsedProviders).toContain("anthropic");
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\.5\s*▾\s*$/);
});

test("with the selection's group collapsed, the trigger still shows the full label after reload", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("commander.prefs.v2", JSON.stringify({ collapsedProviders: ["anthropic"] }));
  });
  await stubModels(page, FLAT);
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\.5\s*▾\s*$/);
});

test("keyboard with the selection's group collapsed only lands on visible rows; Escape keeps the selection", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("commander.prefs.v2", JSON.stringify({ collapsedProviders: ["anthropic"] }));
  });
  await stubModels(page, FLAT);
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").focus();
  await page.keyboard.press("ArrowDown");  // open
  await expect(sel.locator('li[role="option"]')).toHaveCount(2);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");  // clamp at the end
  await expect(sel.locator("button.hi")).toHaveCount(1);
  await expect(sel.locator("button.hi")).toHaveText("qwen (needs key)");
  await page.keyboard.press("Escape");
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\.5\s*▾\s*$/);
  await page.keyboard.press("ArrowDown");  // reopen
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");  // clamp at the first visible row
  await expect(sel.locator("button.hi")).toHaveText("glm-5.3");
  await page.keyboard.press("Enter");
  await expect(sel.locator("button.trigger")).toHaveText(/^Ollama · glm-5\.3\s*▾\s*$/);
});

const TRIGGER_OPUS_5 = /^Anthropic · Opus 5\s*▾\s*$/;

// Arrow keys must not land on a header, and must not reach rows inside a
// collapsed group. This is the bug class a markup-only assertion misses.
// The header click deliberately does NOT re-focus the trigger: the header
// must not take focus off it in the first place.
test("keyboard navigation still works after clicking a header, and skips collapsed rows", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  await sel.locator('li[role="presentation"]', { hasText: "Ollama Cloud" }).locator("button").click();
  await expect(sel.locator("button.trigger")).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");  // past the end of the 2 visible rows
  await expect(sel.locator("button.hi")).toHaveText("Opus 5");
  await page.keyboard.press("Enter");
  await expect(sel.locator("button.trigger")).toHaveText(TRIGGER_OPUS_5);
});

// Crossing an expanded header: the highlight and the commit must both follow
// the SELECTABLE index, not the render-row index (which counts headers).
test("arrowing across an expanded header lands on the next group's first row", async ({ page }) => {
  await stubModels(page, FLAT);
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").focus();
  await page.keyboard.press("ArrowDown");  // open, hi = Opus 5.5
  await page.keyboard.press("ArrowDown");  // Opus 5
  await expect(sel.locator("button.hi")).toHaveText("Opus 5");
  await page.keyboard.press("ArrowDown");  // across the Ollama header
  await expect(sel.locator("button.hi")).toHaveCount(1);
  await expect(sel.locator("button.hi")).toHaveText("glm-5.3");
  await page.keyboard.press("Enter");
  await expect(sel.locator("button.trigger")).toHaveText(/^Ollama · glm-5\.3\s*▾\s*$/);
});

test("group headers expose their state to assistive tech", async ({ page }) => {
  await stubModels(page, FLAT);
  const sel = await openPicker(page);
  const hdr = sel.locator('li[role="presentation"]', { hasText: "Ollama Cloud" }).locator("button");
  await expect(hdr).toHaveAttribute("aria-expanded", "true");
  await expect(hdr.locator("[data-state]")).toHaveAttribute("aria-hidden", "true");
  await hdr.click();
  await expect(hdr).toHaveAttribute("aria-expanded", "false");
});

test("a single-provider catalog renders no headers and keeps full labels", async ({ page }) => {
  await stubModels(page, FLAT.filter((m) => m.provider === "anthropic"));
  const sel = await openPicker(page);
  await expect(sel.locator('li[role="presentation"]')).toHaveCount(0);
  await expect(sel.getByRole("option", { name: "Anthropic · Opus 5.5" })).toBeVisible();
});

test("with every group collapsed, ArrowDown + Enter leaves the selection unchanged", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("commander.prefs.v2", JSON.stringify({ collapsedProviders: ["anthropic", "ollama-cloud"] }));
  });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await stubModels(page, FLAT);
  await page.goto("/?nointro");
  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").focus();
  await page.keyboard.press("ArrowDown");  // open
  await expect(sel.locator('li[role="option"]')).toHaveCount(0);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(sel.locator("button.trigger")).toHaveText(/^Anthropic · Opus 5\.5\s*▾\s*$/);
  expect(errors).toEqual([]);
});
