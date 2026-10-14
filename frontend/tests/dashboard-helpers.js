// Wails bindings/runtime stubs. Live sessions, timeline and progress are
// seeded per test; dash:tick is fired through __handlers.
export async function stubDash(page, { live = [], timeline = {}, windows = [{ windowID: "@1", name: "proj" }], stats, projects, progress } = {}) {
  await page.addInitScript(({ live, timeline, windows, stats, projects, progress }) => {
    window.__calls = [];
    window.__handlers = {};
    window.runtime = Object.assign(window.runtime || {}, {
      EventsOnMultiple: (name, cb) => { (window.__handlers[name] ||= []).push(cb); return () => {}; },
      EventsOff: () => {}, EventsEmit: () => {},
    });
    const st = stats || {
      "@1": { sessionID: "s1", title: "Fix VAT", activity: "tool", tool: "Edit", target: "app.go", model: "claude-opus-5-5",
        provider: "anthropic", status: "active", cwd: "/Users/me/proj", contextTokens: 1000, contextWindow: 200000, band: "green", turns: 3 },
    };
    window.go = { main: { App: {
      Config: async () => [{ id: "claude-opus-5-5", label: "Opus 5.5", routed: false, ready: true }],
      ListSessions: async () => windows,
      SessionStats: async (id) => st[id] || {},
      SelectSession: async (id) => { window.__calls.push(["SelectSession", id]); },
      WSPort: async () => 0, WSToken: async () => "",
      LedgerStatus: async () => ({ enabled: false, storeTitles: true, readOnly: false, excluded: [], progress: { files: 0, total: 0, bytes: 0, done: true }, lastError: "", dir: "" }),
      CodeView: async (key) => { window.__calls.push(["CodeView", key]); return window.__code || { key, locked: true, folders: [], files: [], slabs: [] }; },
      ScanProject: async (key) => { window.__calls.push(["ScanProject", key]); },
      CityView: async () => { window.__calls.push(["CityView"]); return window.__city || { locked: false, districts: [] }; },
      DashSnapshot: async () => ({ sessions: live, projects: projects || [{ key: "k1", label: "proj", root: "/Users/me/proj" }], timeline }),
      Progress: async () => progress || ({ commander: { level: 3 }, projects: [{ key: "k1", rank: "Station" }], live: [{ sessionID: "s1", level: 7, title: "Adept" }], locked: false }),
      PowerState: async () => ({ lowPower: false }),
      PlanUsage: async () => window.__usage || null,
      RaidPackInfo: async () => window.__raidPack || { dir: "", name: "", files: {}, heroines: {}, warnings: [] },
      SetLiveFocus: async () => {},
    } } };
  }, { live, timeline, windows, stats, projects, progress });
}

export const S1 = { id: "s1", title: "Fix VAT", state: "tool", tool: "Edit", target: "app.go", cwd: "/Users/me/proj", crew: [], sinceMs: Date.now() - 5000 };

export async function pickMode(page, id) {
  await page.getByTestId("dash-mode-switch").click();
  await page.getByTestId("dash-mode-" + id).click();
}
