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
      // Quota sampler: window.__quota (QuotaNow) or derived from __usage; history in __quotaHist[provider].
      QuotaNow: async () => window.__quota || { providers: [
        { provider: "anthropic", label: "Anthropic", supported: true, at: "", error: "",
          windows: Object.entries(window.__usage || {}).filter(([, w]) => typeof w?.utilization === "number")
            .map(([key, w]) => ({ key, label: key === "five_hour" ? "Session" : key, weekly: key.startsWith("seven_day"), util: w.utilization, resetsAt: w.resets_at || "" })) },
        { provider: "bedrock", label: "Bedrock", supported: false, windows: [] },
        { provider: "ollama-cloud", label: "Ollama Cloud", supported: false, windows: [] },
        { provider: "opencode-go", label: "OpenCode Go", supported: false, windows: [] }] },
      QuotaRefresh: async () => { window.__calls.push(["QuotaRefresh"]); return window.go.main.App.QuotaNow(); },
      QuotaHistory: async (p) => { window.__calls.push(["QuotaHistory", p]); return window.__quotaHist?.[p] || { windows: [], locked: false }; },
      RaidPackInfo: async () => window.__raidPack || { dir: "", name: "", files: {}, heroines: {}, warnings: [] },
      SetLiveFocus: async () => {},
      // Al Asturlab: fixtures in window.__ast (see asturlab-fixtures.js), calls in __calls.
      Stats: async (q) => { window.__calls.push(["Stats", q]); const f = window.__ast; if (!f) return { rows: [], pricedAt: "2026-10-06", stale: false };
        return { rows: (f.rows[q.groupBy + "|" + q.metric] || f.rows[q.groupBy + (q.split ? ":" + q.split : "")] || []).map((r) => ({ ...r })), pricedAt: f.pricedAt || "2026-10-06", stale: !!f.stale }; },
      StatsOverview: async (from, to) => { window.__calls.push(["StatsOverview", from, to]); return window.__ast?.overview || { locked: true, overview: { cur: {}, prev: {} } }; },
      StatsModels: async (from, to, project) => { window.__calls.push(["StatsModels", from, to, project]); return window.__ast?.models || []; },
      ProjectMetrics: async (key) => { window.__calls.push(["ProjectMetrics", key]); return window.__metrics || { files: {}, snapshots: [] }; },
      AsturlabSettings: async () => window.__astSettings || {},
      SetAsturlabSettings: async (s) => { window.__calls.push(["SetAsturlabSettings", s]); window.__astSettings = s; },
      ExportStats: async (...a) => { window.__calls.push(["ExportStats", ...a]); return "/tmp/x"; },
      // Al Asturlab AI (phase 3): __brief (Briefing view), __briefRun (RunBriefing result), __askAns (AskStats), __wrapped[year], __sessions.
      Briefing: async () => { window.__calls.push(["Briefing"]); return window.__brief || { locked: false, week: "2026-11-09", brief: null, running: false, disclosure: "Sends totals to haiku.", askDisclosure: "Sends your question to haiku." }; },
      RunBriefing: async () => { window.__calls.push(["RunBriefing"]); const v = window.__briefRun || { locked: false, week: "2026-11-09", brief: { week: "2026-11-09", headline: "Busy week", points: [{ text: "Tokens up 20%", chart: "spend" }, { text: "Quiet Sunday", chart: "" }] }, disclosure: "Sends totals to haiku." }; window.__brief = v; return v; },
      AskStats: async (q) => { window.__calls.push(["AskStats", q]); return window.__askAns || { question: q, answer: "Most tokens went to vat.", queries: [{ from: "2026-10-01", to: "2026-10-31", groupBy: "project", metric: "tokens" }], results: [{ rows: [{ key: "k1", label: "vat", value: 900 }, { key: "k2", label: "web", value: 300 }] }], disclosure: "Sends your question to haiku." }; },
      Wrapped: async (y) => { window.__calls.push(["Wrapped", y]); return window.__wrapped?.[y] || { year: y, empty: true, years: [y], topProjects: [], models: [] }; },
      SaveWrappedPNG: async (u) => { window.__calls.push(["SaveWrappedPNG", u.slice(0, 22)]); return "/tmp/w.png"; },
      ProjectSessions: async (...a) => { window.__calls.push(["ProjectSessions", ...a]); return window.__sessions || []; },
    } } };
  }, { live, timeline, windows, stats, projects, progress });
}

export const S1 = { id: "s1", title: "Fix VAT", state: "tool", tool: "Edit", target: "app.go", cwd: "/Users/me/proj", crew: [], sinceMs: Date.now() - 5000 };

export async function pickMode(page, id) {
  await page.getByTestId("dash-mode-switch").click();
  await page.getByTestId("dash-mode-" + id).click();
}
