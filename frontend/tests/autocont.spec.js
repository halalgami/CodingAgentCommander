import { test, expect } from "@playwright/test";
import { stubDash } from "./dashboard-helpers.js";

// Models drawer surface: one routed (default on), one native (default off).
async function stubModels(page, { ready = true } = {}) {
  await page.addInitScript((ready) => {
    const state = { catalog: [
      { id: "glm", label: "Ollama · glm-5.3", provider: "ollama-cloud", routed: true, autoContinue: true, autoContinueSet: false, autoContinueReady: ready },
      { id: "claude-opus-5", label: "Anthropic · Opus 5", provider: "anthropic", routed: false, autoContinue: false, autoContinueSet: false, autoContinueReady: ready },
    ] };
    window.__autoCalls = [];
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ListProviders: async () => [], KeyStatus: async () => [],
      Models: async () => state.catalog.map((m) => ({ ...m })),
      Config: async () => state.catalog.map((m) => ({ id: m.id, label: m.label, routed: m.routed, ready: true })),
      ListSessions: async () => [], ListProjects: async () => [],
      SetModelAutoContinue: async (id, on) => {
        window.__autoCalls.push([id, on]);
        const m = state.catalog.find((x) => x.id === id);
        m.autoContinue = on; m.autoContinueSet = true;
      },
    });
  }, ready);
}

async function openModels(page) {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await page.getByTestId("palette-input").fill("models");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("drawer-models")).toBeVisible();
}

test("toggle shows the default dimmed and saves an explicit value", async ({ page }) => {
  await stubModels(page);
  await openModels(page);
  const glm = page.getByTestId("model-autocont-glm");
  await expect(glm).toHaveAttribute("aria-pressed", "true");
  await expect(glm).toHaveClass(/default/);
  await expect(glm).toHaveAttribute("title", /\(default\)$/);
  await expect(page.getByTestId("model-autocont-claude-opus-5")).toHaveAttribute("aria-pressed", "false");
  await glm.click();
  await expect(glm).toHaveAttribute("aria-pressed", "false");
  await expect(glm).not.toHaveClass(/default/);
  expect(await page.evaluate(() => window.__autoCalls)).toEqual([["glm", false]]);
});

test("toggle is disabled on a Claude Code without http hooks", async ({ page }) => {
  await stubModels(page, { ready: false });
  await openModels(page);
  const glm = page.getByTestId("model-autocont-glm");
  await expect(glm).toBeDisabled();
  await expect(glm).toHaveAttribute("title", "Needs Claude Code 2.1.210 or newer");
});

test("card shows ↻ N only while the prompt has auto-continues", async ({ page }) => {
  const base = { sessionID: "s1", model: "glm", provider: "ollama-cloud", status: "active", cwd: "/Users/me/proj",
    contextTokens: 1000, contextWindow: 200000, band: "green", turns: 3 };
  await stubDash(page, { stats: { "@1": { ...base, autoContinues: 2 } } });
  await page.goto("/?nointro");
  const chip = page.getByTestId("card-autocont");
  await expect(chip).toHaveText("↻ 2");
  await expect(chip).toHaveAttribute("title", "Auto-continued 2 times on this prompt");
});

test("card hides the chip at 0", async ({ page }) => {
  await stubDash(page, { stats: { "@1": { sessionID: "s1", model: "glm", provider: "ollama-cloud", status: "active",
    cwd: "/Users/me/proj", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1, autoContinues: 0 } } });
  await page.goto("/?nointro");
  await expect(page.getByTestId("session-card").first()).toBeVisible();
  await expect(page.getByTestId("card-autocont")).toHaveCount(0);
});

test("gave-up event shows the toast", async ({ page }) => {
  await stubDash(page);
  await page.goto("/?nointro");
  await page.waitForFunction(() => (window.__handlers["session:autocont-gaveup"] || []).length > 0);
  await page.evaluate(() => window.__handlers["session:autocont-gaveup"].forEach((cb) => cb({ windowID: "@1", model: "Ollama · glm-5.3", why: "stuck" })));
  await expect(page.getByTestId("toast")).toContainText("Ollama · glm-5.3 seems stuck — over to you");
});
