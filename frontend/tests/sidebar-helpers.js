
// Go is stubbed: SidebarLayout returns window.__layout, SetSidebarLayout
// records into window.__layouts and becomes the next SidebarLayout answer
// (what the real store does). ListSessions stays unstubbed: refresh()
// swallows the error, so the seeded sessions survive.
export async function boot(page, { sessions = [["@1", "W1"], ["@2", "W2"], ["@3", "W3"]], layout = null, stats = {} } = {}) {
  await page.addInitScript((layout) => {
    window.go = window.go || {}; window.go.main = window.go.main || {};
    window.__layout = layout ?? { groups: [], order: { ungrouped: [] } };
    window.__layouts = []; window.__selected = []; window.__killed = []; window.__dropped = [];
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ScreenshotSupport: async () => false,
      SelectSession: async (id) => { window.__selected.push(id); return null; },
      KillSession: async (id) => { window.__killed.push(id); return null; },
      InsertDroppedPaths: async (w, p) => { window.__dropped.push([w, p]); return { inserted: [], rejected: [] }; },
      SidebarLayout: async () => JSON.parse(JSON.stringify(window.__layout)),
      SetSidebarLayout: async (l) => { window.__layouts.push(l); window.__layout = JSON.parse(JSON.stringify(l)); return null; },
    });
  }, layout);
  await page.goto("/?nointro");
  await page.evaluate(([sessions, stats]) => {
    window.__app.sessions = sessions.map(([windowID, name]) => ({ windowID, name }));
    window.__app.stats = stats;
  }, [sessions, stats]);
}

export const last = (page) => page.evaluate(() => window.__layouts.at(-1));
export const card = (page, id) => page.locator(`[data-testid="session-card"][data-window-id="${id}"]`);
export const order = (page) => page.locator('[data-testid="session-card"]').evaluateAll((els) => els.map((e) => e.dataset.windowId));

