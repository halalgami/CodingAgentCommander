import { test, expect } from "@playwright/test";

// Wails bindings don't exist in a plain browser, so stub window.go's App with an
// in-memory provider/catalog before any app script runs.
async function stubOllama(page, { keySet = false } = {}) {
  await page.addInitScript((seed) => {
    const state = { defined: false, keySet: seed.keySet, catalog: [] };
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ListProviders: async () => [
        {
          type: "ollama-cloud",
          defined: state.defined,
          active: state.defined && state.keySet,
          apiBase: "https://ollama.com",
          region: "",
          modelCnt: state.catalog.length,
        },
      ],
      KeyStatus: async () => (state.defined ? [{ env: "OLLAMA_API_KEY", set: state.keySet, optional: false }] : []),
      AddProvider: async (type) => { if (type === "ollama-cloud") state.defined = true; },
      RemoveProvider: async () => { state.defined = false; },
      SetKey: async () => { state.keySet = true; },
      ClearKey: async () => { state.keySet = false; },
      DiscoverOllamaModels: async () => [
        { id: "ollama-glm-5.3", label: "Ollama · glm-5.3", upstream: "ollama_chat/glm-5.3" },
        { id: "ollama-gpt-oss:120b", label: "Ollama · gpt-oss:120b", upstream: "ollama_chat/gpt-oss:120b" },
      ],
      AddModel: async (m) => { state.catalog.push(m); window.__added = state.catalog.slice(); },
      Models: async () => state.catalog.slice(),
      Config: async () => state.catalog.map((m) => ({ id: m.id, label: m.label, routed: true, ready: true })),
      ListSessions: async () => [],
      ListProjects: async () => [],
    });
  }, { keySet });
}

test("define Ollama, discover, add a model with its colon intact", async ({ page }) => {
  await stubOllama(page);
  // ?nointro skips the boot animation, as every other spec does.
  await page.goto("/?nointro");

  // Providers drawer: define, then the key input must render (it did not when
  // envsFor was a binary ternary).
  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();
  await page.getByTestId("define-ollama-cloud").click();
  await expect(page.getByTestId("key-input-OLLAMA_API_KEY")).toBeVisible();
  await page.keyboard.press("Escape");

  // Models drawer: the discover testid must be provider-specific.
  await page.getByTestId("open-models").click();
  await expect(page.getByTestId("drawer-models")).toBeVisible();
  await page.getByTestId("discover-ollama-cloud").click();

  const row = page.locator(".discrow", { hasText: "gpt-oss:120b" });
  await expect(row).toBeVisible();
  await row.locator("input[type=checkbox]").check();
  await page.getByRole("button", { name: "Add selected" }).click();

  // The colon survives: the backend derives the id, the frontend must not mangle it.
  const added = await page.evaluate(() => window.__added);
  expect(added.map((m) => m.id)).toContain("ollama-gpt-oss:120b");
  expect(added.find((m) => m.id === "ollama-gpt-oss:120b").upstream).toBe("ollama_chat/gpt-oss:120b");
});
