import { test, expect } from "@playwright/test";

// Wails bindings don't exist in a plain browser, so stub window.go's App with an
// in-memory history before any app script runs. This lets the data-driven drawer
// be exercised end-to-end (render, search, pin, remove, prefill).
async function stubHistory(page, rows) {
  await page.addInitScript((seed) => {
    const store = seed.slice();
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ListProjects: async () => store.slice(),
      PinProject: async (folder, pinned) => {
        const e = store.find((p) => p.folder === folder); if (e) e.pinned = pinned;
      },
      RemoveProject: async (folder) => {
        const i = store.findIndex((p) => p.folder === folder); if (i >= 0) store.splice(i, 1);
      },
      RenameProject: async () => {},
      ImportProjects: async () => {},
      Config: async () => ([
        { id: "claude-opus-4-8", label: "Opus 4.8", routed: false, ready: true },
        { id: "claude-sonnet-5", label: "Sonnet 5", routed: false, ready: true },
      ]),
      LaunchSession: async (folder, modelID, rc) => {
        window.__launches = window.__launches || [];
        window.__launches.push({ folder, modelID, rc });
        return { windowID: "w1" };
      },
      ListSessions: async () => [],
    });
  }, rows);
}

const ROWS = [
  { folder: "/Users/me/alpha", label: "alpha", lastModelID: "claude-opus-4-8", lastOpened: 3, openCount: 5, pinned: true, missing: false },
  { folder: "/Users/me/beta", label: "beta", lastModelID: "claude-opus-4-8", lastOpened: 2, openCount: 1, pinned: false, missing: false },
  { folder: "/Users/me/gone", label: "gone", lastModelID: "claude-opus-4-8", lastOpened: 1, openCount: 1, pinned: false, missing: true },
];

test("history drawer opens from the footer and lists projects", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await expect(page.getByTestId("drawer-history")).toBeVisible();
  await expect(page.getByTestId("history-row-/Users/me/alpha")).toBeVisible();
  await expect(page.getByTestId("history-row-/Users/me/beta")).toBeVisible();
});

test("history search filters the list", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-search").fill("beta");
  await expect(page.getByTestId("history-row-/Users/me/beta")).toBeVisible();
  await expect(page.getByTestId("history-row-/Users/me/alpha")).not.toBeVisible();
});

test("removing a row drops it from the list", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-remove-/Users/me/beta").click();
  await expect(page.getByTestId("history-row-/Users/me/beta")).not.toBeVisible();
});

test("clicking a history row opens the launch-confirm modal with the full path", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/alpha").click();
  const modal = page.getByTestId("launch-confirm");
  await expect(modal).toBeVisible();
  await expect(page.getByTestId("launch-confirm-path")).toHaveText("/Users/me/alpha");
});

test("cancel closes the modal without launching", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/alpha").click();
  await page.getByTestId("launch-confirm-cancel").click();
  await expect(page.getByTestId("launch-confirm")).not.toBeVisible();
  const launches = await page.evaluate(() => window.__launches || []);
  expect(launches.length).toBe(0);
});

test("confirm launches with the chosen folder + model", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/alpha").click();
  await page.getByTestId("launch-confirm-go").click();
  await expect(page.getByTestId("launch-confirm")).not.toBeVisible();
  const launches = await page.evaluate(() => window.__launches || []);
  expect(launches.length).toBe(1);
  expect(launches[0].folder).toBe("/Users/me/alpha");
  expect(launches[0].modelID).toBe("claude-opus-4-8"); // alpha's lastModelID
});

test("changing the model then launching carries the chosen model", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/alpha").click();
  // pick a DIFFERENT model than alpha's default (opus) so the assertion can't
  // be satisfied by the pre-existing selection
  const select = page.getByTestId("launch-confirm-model");
  await select.locator("button.trigger").click();
  await select.getByText("Sonnet 5").click();
  await page.getByTestId("launch-confirm-go").click();
  await expect(page.getByTestId("launch-confirm")).not.toBeVisible();
  const launches = await page.evaluate(() => window.__launches || []);
  expect(launches.length).toBe(1);
  expect(launches[0].modelID).toBe("claude-sonnet-5");
});

test("pressing Enter inside the model dropdown does NOT launch", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/alpha").click();
  // focus the model dropdown trigger and press Enter (opens it) — this must not
  // bubble to the modal and fire a launch
  await page.getByTestId("launch-confirm-model").locator("button.trigger").focus();
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("launch-confirm")).toBeVisible(); // still open
  const launches = await page.evaluate(() => window.__launches || []);
  expect(launches.length).toBe(0);
});

test("missing-folder row disables launch and warns", async ({ page }) => {
  await stubHistory(page, ROWS);
  await page.goto("/?nointro");
  await page.getByTestId("open-history").click();
  await page.getByTestId("history-open-/Users/me/gone").click(); // ROWS 'gone' has missing:true
  await expect(page.getByTestId("launch-confirm-missing")).toBeVisible();
  await expect(page.getByTestId("launch-confirm-go")).toBeDisabled();
});
