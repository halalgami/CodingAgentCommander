import { test, expect } from "@playwright/test";

// Stubs the native-model surface: the catalog, and a RefreshModels binding
// whose behaviour each test controls.
async function stubNative(page, { refresh } = {}) {
  await page.addInitScript((seed) => {
    const state = { catalog: [{ id: "claude-opus-5", label: "Anthropic · Opus 5", provider: "anthropic" }] };
    window.__refreshCalls = 0;
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ListProviders: async () => [],
      KeyStatus: async () => [],
      Models: async () => state.catalog.slice(),
      Config: async () => state.catalog.map((m) => ({ id: m.id, label: m.label, routed: false, ready: true })),
      ListSessions: async () => [],
      ListProjects: async () => [],
      AddModel: async (m) => { state.catalog.push(m); },
      RemoveModel: async (id) => { state.catalog = state.catalog.filter((m) => m.id !== id); },
      RefreshModels: async () => {
        window.__refreshCalls++;
        if (seed.refresh === "fail") throw new Error("no Anthropic credential: sign in with `claude` once");
        // The real backend adds the model and then emits models:updated, which
        // App.svelte turns into a reloadModels(). Mirror both halves.
        state.catalog.push({ id: "claude-opus-5-5", label: "Anthropic · Opus 5.5", provider: "anthropic" });
      },
    });
  }, { refresh });
}

async function openModels(page) {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await page.getByTestId("palette-input").fill("models");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("drawer-models")).toBeVisible();
}

// The whole point of the feature: a model released after this build lands in
// the picker without the user editing TOML or reinstalling.
test("checking for new models adds one released since the build", async ({ page }) => {
  await stubNative(page);
  await openModels(page);
  await expect(page.getByTestId("drawer-models")).not.toContainText("Opus 5.5");

  await page.getByTestId("refresh-native").click();
  await expect(page.getByTestId("drawer-models")).toContainText("Opus 5.5");
  expect(await page.evaluate(() => window.__refreshCalls)).toBe(1);
});

// A missing or expired Claude Code login is the common failure and the one the
// user can actually fix, so it has to reach them — not vanish into a console.
test("a failed check reports why, inline", async ({ page }) => {
  await stubNative(page, { refresh: "fail" });
  await openModels(page);
  await page.getByTestId("refresh-native").click();
  await expect(page.getByTestId("add-model-error")).toContainText("credential");
  // The button has to come back, or one failure disables the feature for good.
  await expect(page.getByTestId("refresh-native")).toBeEnabled();
});

async function stubCatalog(page, cat) {
  await page.addInitScript((cat) => {
    window.go = window.go || {}; window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      Models: async () => cat,
      Config: async () => cat.map((m) => ({ ...m, routed: false, ready: true, default: false })),
      ListProviders: async () => [], KeyStatus: async () => [],
      ListSessions: async () => [], ListProjects: async () => [],
    });
  }, cat);
}

test("the drawer catalog groups by provider and shares collapse state", async ({ page }) => {
  await stubCatalog(page, [
    { id: "claude-opus-5-5", label: "Anthropic · Opus 5.5", provider: "anthropic" },
    { id: "ollama-glm", label: "Ollama · glm-5.3", provider: "ollama-cloud" },
  ]);
  await openModels(page);
  const drawer = page.getByTestId("drawer-models");
  const hdrs = drawer.locator(".catalog .grouphdr");
  await expect(hdrs).toHaveCount(2);
  await expect(hdrs.first()).toContainText("Anthropic");
  await expect(hdrs.first().locator(".count")).toHaveText("1");
  // Prefix stripped under the header; row shows exactly the bare name.
  const rows = drawer.locator(".catalog li.model");
  await expect(rows).toHaveCount(2);
  await expect(rows.first().locator("span").first()).toHaveText("Opus 5.5");
  await expect(rows.nth(1).locator("span").first()).toHaveText("glm-5.3");
  // Collapsing hides rows, count stays.
  const ollama = hdrs.filter({ hasText: "Ollama Cloud" });
  await ollama.locator("button").click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first().locator("span").first()).toHaveText("Opus 5.5");
  await expect(ollama.locator(".count")).toHaveText("1");
  // And it expands again.
  await ollama.locator("button").click();
  await expect(rows).toHaveCount(2);
});

test("a single-provider drawer catalog has no header", async ({ page }) => {
  await stubCatalog(page, [
    { id: "a", label: "Anthropic · Opus 5.5", provider: "anthropic" },
    { id: "b", label: "Anthropic · Sonnet 5.5", provider: "anthropic" },
  ]);
  await openModels(page);
  const drawer = page.getByTestId("drawer-models");
  await expect(drawer.locator(".catalog li.model")).toHaveCount(2);
  await expect(drawer.locator(".catalog .grouphdr")).toHaveCount(0);
});

// Spec-mandated: one stored fold, two surfaces. Selection is Anthropic (the
// first model), so the Ollama group is not pinned open in the picker.
test("the drawer and the launch picker agree on collapse state", async ({ page }) => {
  await stubCatalog(page, [
    { id: "claude-opus-5-5", label: "Anthropic · Opus 5.5", provider: "anthropic" },
    { id: "ollama-glm", label: "Ollama · glm-5.3", provider: "ollama-cloud" },
    { id: "ollama-qwen", label: "Ollama · qwen", provider: "ollama-cloud" },
  ]);
  await openModels(page);
  const drawer = page.getByTestId("drawer-models");
  const dbtn = drawer.locator(".catalog .grouphdr").filter({ hasText: "Ollama Cloud" }).locator("button");
  await expect(dbtn.locator("[data-state]")).toHaveAttribute("data-state", "expanded");
  await dbtn.click();
  await expect(dbtn.locator("[data-state]")).toHaveAttribute("data-state", "collapsed");
  await dbtn.click();
  await expect(dbtn.locator("[data-state]")).toHaveAttribute("data-state", "expanded");
  await dbtn.click();
  await expect(drawer.locator(".catalog li.model")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveCount(0);

  const sel = page.getByTestId("model-select");
  await sel.locator("button.trigger").click();
  const hdr = sel.locator('li[role="presentation"]', { hasText: "Ollama Cloud" });
  await expect(hdr.locator(".count")).toHaveText("2");
  await expect(hdr.locator("button")).toHaveAttribute("aria-expanded", "false");
  await expect(hdr.locator("[data-state]")).toHaveAttribute("data-state", "collapsed");
  await expect(sel.getByRole("option", { name: /glm|qwen/ })).toHaveCount(0);
  await expect(sel.getByRole("option", { name: "Opus 5.5" })).toBeVisible();
});
