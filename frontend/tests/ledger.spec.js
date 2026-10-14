import { test, expect } from "@playwright/test";

// Wails bindings and runtime don't exist in a plain browser: stub both before
// any app script runs. Events are captured so tests can fire them.
async function stub(page, { title = "Fix VAT", status } = {}) {
  await page.addInitScript(({ title, status }) => {
    window.__calls = [];
    window.__handlers = {};
    window.runtime = Object.assign(window.runtime || {}, {
      EventsOnMultiple: (name, cb) => { (window.__handlers[name] ||= []).push(cb); return () => {}; },
      EventsOff: () => {},
      EventsEmit: () => {},
    });
    let st = status;
    window.go = { main: { App: {
      Config: async () => [{ id: "claude-opus-4-8", label: "Opus 4.8", routed: false, ready: true }],
      ListSessions: async () => [{ windowID: "@1", name: "proj" }],
      SessionStats: async () => ({
        sessionID: "s1", title, activity: "tool", tool: "Edit", target: "app.go", unread: false,
        model: "claude-opus-4-8", provider: "anthropic", status: "active", cwd: "/Users/me/proj",
        contextTokens: 1000, contextWindow: 200000, band: "ok", turns: 3, uptimeSeconds: 60,
      }),
      LedgerStatus: async () => st,
      SetLedgerConsent: async (on, titles) => {
        window.__calls.push(["SetLedgerConsent", on, titles]);
        st = { ...st, enabled: on, storeTitles: titles };
        return st;
      },
      DashSnapshot: async () => ({ sessions: [], projects: [] }),
      SetLiveFocus: async (f) => window.__calls.push(["SetLiveFocus", f]),
    } } };
  }, { title, status: status ?? { enabled: false, storeTitles: true, readOnly: false, excluded: [], progress: { files: 0, total: 0, bytes: 0, done: false }, lastError: "", dir: "~/.config/commander/ledger" } });
}

test("card shows title and live activity", async ({ page }) => {
  await stub(page);
  await page.goto("/?nointro");
  await expect(page.getByTestId("session-title")).toHaveText("Fix VAT");
  await expect(page.getByTestId("session-activity")).toHaveText("Edit app.go");
});

test("malicious title renders literally", async ({ page }) => {
  const evil = `<img src=x onerror="window.__xss=1">`;
  await stub(page, { title: evil });
  await page.goto("/?nointro");
  await expect(page.getByTestId("session-title")).toHaveText(evil);
  expect(await page.evaluate(() => window.__xss)).toBeUndefined();
  expect(await page.locator(".card img").count()).toBe(0);
});

test("history opt-in calls the backend and shows progress", async ({ page }) => {
  await stub(page);
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await expect(page.getByTestId("ledger-explainer")).toContainText("never leaves this Mac");
  await page.getByTestId("ledger-toggle").check();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SetLedgerConsent", true, true]);
  await page.evaluate(() => window.__handlers["ledger:progress"].forEach((cb) => cb({ files: 340, total: 1113, bytes: 1, done: false })));
  await expect(page.getByTestId("ledger-progress")).toContainText("340 / 1,113");
});

test("history can be deleted while recording is off", async ({ page }) => {
  await stub(page, { status: { enabled: false, hasData: true, storeTitles: true, readOnly: false, excluded: [], progress: { files: 0, total: 0, bytes: 0, done: true }, lastError: "", dir: "~/.config/commander/ledger" } });
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await expect(page.getByTestId("ledger-toggle")).not.toBeChecked();
  await expect(page.getByTestId("ledger-delete")).toBeVisible();
});

test("activity line flags a hotspot", async ({ page }) => {
  await stub(page);
  await page.addInitScript(() => {
    const orig = window.go.main.App.SessionStats;
    window.go.main.App.SessionStats = async (id) => ({ ...(await orig(id)), hotspot: true });
  });
  await page.goto("/?nointro");
  await expect(page.getByTestId("session-activity")).toHaveText("Edit app.go ⚠ hotspot");
});
