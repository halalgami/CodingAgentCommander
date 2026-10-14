// Dashboard state: live sessions, timelines, projects and progress, fed by a
// DashSnapshot on show and dash:tick while shown (spec §4.3).
import { tick } from "svelte";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { DashSnapshot, Progress, PowerState, CityView, CodeView } from "../../../wailsjs/go/main/App.js";
import { app, select } from "../stores.svelte.js";
import { prefs, setPref } from "../prefs.svelte.js";
import { appendEvents } from "./timeline.js";


export const dash = $state({ live: [], timeline: {}, projects: [], progress: null, lowPower: false, city: { locked: false, districts: [] }, zoom: null });

export const isHome = () => app.view === "home" || !app.sessionKey;

export async function loadDash() {
  try {
    const snap = await DashSnapshot();
    dash.live = snap.sessions || [];
    dash.projects = snap.projects || [];
    dash.timeline = snap.timeline || {};
  } catch {}
  try { dash.progress = await Progress(); } catch {}
  try {
    dash.city = await CityView();
    // A project forgotten while the dashboard was hidden: leave its CodeCity.
    if (dash.zoom && !(dash.city.districts || []).some((d) => d.key === dash.zoom.key)) zoomOut();
  } catch {}
}

function applyTick(t) {
  if (!isHome()) return; // hidden: drop; loadDash() re-syncs on show
  if (t.sessions?.length) {
    const byId = new Map(dash.live.map((s) => [s.id, s]));
    for (const s of t.sessions) byId.set(s.id, s);
    dash.live = [...byId.values()];
  }
  if (t.events?.length) dash.timeline = appendEvents(dash.timeline, t.events, Date.now());
  if (t.removed?.length) {
    const gone = new Set(t.removed);
    dash.live = dash.live.filter((s) => !gone.has(s.id));
  }
}

export function goHome() {
  app.view = "home";
  // Keys typed at home must never reach the terminal underneath.
  if (document.activeElement && document.activeElement !== document.body) document.activeElement.blur();
  loadDash();
}

export function toggleHome() {
  if (isHome() && app.sessionKey) {
    app.view = "terminal";
    // Back in the terminal, typing goes straight to it (no click needed) —
    // unless something else holds focus, such as an open drawer or dialog.
    const a = document.activeElement;
    const free = !a || a === document.body || !a.isConnected || !!a.closest?.('[data-testid="dash-home"]');
    if (free) tick().then(() => document.querySelector(".pane .xterm-helper-textarea")?.focus());
  } else goHome();
}

// CodeCity zoom (phase 5b): one project's code, fetched on zoom-in and
// refreshed (debounced) when that project changes.
let codeTimer = 0, codeSeq = 0;
async function fetchCode(key) {
  const seq = ++codeSeq;
  try {
    const v = await CodeView(key);
    if (seq === codeSeq && dash.zoom?.key === key) dash.zoom = { ...dash.zoom, view: v };
  } catch {}
}
export function zoomInto(key, label, origin = null) {
  clearTimeout(codeTimer);
  codeTimer = 0;
  dash.zoom = { key, label, view: null, origin };
  return fetchCode(key);
}
// refreshCode keeps the current view on screen until the new one arrives, at
// most once per 2 s however often the project changes (a throttle, so steady
// activity cannot starve it).
function refreshCode(key) {
  if (codeTimer) return;
  codeTimer = setTimeout(() => {
    codeTimer = 0;
    if (dash.zoom?.key === key) fetchCode(key);
  }, 2000);
}
export function zoomOut() {
  clearTimeout(codeTimer);
  codeTimer = 0;
  codeSeq++;
  dash.zoom = null;
}

export function setMode(mode) {
  zoomOut();
  setPref("dashMode", mode);
  goHome();
}

export async function openRow(row) {
  if (!row.windowID) return;
  await select(row.windowID);
}

export function initDash() {
  try {
    EventsOn("dash:tick", applyTick);
    EventsOn("dash:city", () => {
      if (!isHome() || !(prefs.dashMode === "city" || dash.zoom)) return;
      CityView().then((c) => {
        dash.city = c;
        if (dash.zoom && !(c.districts || []).some((d) => d.key === dash.zoom.key)) zoomOut();
      }).catch(() => {});
    });
    EventsOn("dash:project", (p) => {
      if (dash.zoom?.key === p?.key) refreshCode(p.key);
    });
    EventsOn("dash:achievement", (u) => {
      if (!dash.progress || !u?.projectKey) return; // account unlocks carry no project
      const projects = (dash.progress.projects || []).map((p) => p.key !== u.projectKey ? p
        : { ...p, unlocks: [...(p.unlocks || []).filter((x) => x.id !== u.id), { id: u.id, at: u.at, sessionID: u.sessionID }] });
      dash.progress = { ...dash.progress, projects };
    });
    EventsOn("dash:hero", (h) => {
      if (!dash.progress) return;
      const live = (dash.progress.live || []).filter((x) => x.sessionID !== h.sessionID);
      dash.progress = { ...dash.progress, live: [...live, h] };
    });
  } catch {}
  loadDash();
  const poll = async () => { try { dash.lowPower = (await PowerState()).lowPower; } catch {} };
  poll();
  setInterval(() => { if (isHome()) poll(); }, 60000);
}

export const MODE_GROUPS = ["Views", "Worlds", "Fleet"];
export const modes = [
  { id: "triage", label: "Triage", key: "1", group: "Views" },
  { id: "river", label: "River", key: "2", group: "Views" },
  { id: "city", label: "Al Modon", key: "3", group: "Worlds" },
  { id: "raid", label: "Raid", key: "4", group: "Worlds" },
  { id: "fleet", label: "Fleet Ops", key: "5", group: "Fleet" },
  { id: "cic", label: "CIC", key: "6", group: "Fleet" },
  { id: "sector", label: "Sector Map", key: "7", group: "Fleet" },
];

export const currentMode = () => (modes.some((m) => m.id === prefs.dashMode) ? prefs.dashMode : "triage");
