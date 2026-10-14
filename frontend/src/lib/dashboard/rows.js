// Joins Commander windows, live sessions, projects and progress into one row
// per session, and orders rows by attention (spec §5).
import { projectTint } from "./sigil.js";

const base = (p) => (p || "").replace(/\/+$/, "").split("/").pop() || "";

function projectFor(cwd, projects) {
  let best = null;
  for (const p of projects || []) {
    if (!p.root || !cwd) continue;
    if ((cwd === p.root || cwd.startsWith(p.root + "/")) && (!best || p.root.length > best.root.length)) best = p;
  }
  return best;
}

function row({ live, win, st, projects, progress }) {
  const cwd = live?.cwd || st?.cwd || "";
  const proj = projectFor(cwd, projects);
  const prog = proj && progress?.projects?.find((p) => p.key === proj.key);
  const id = live?.id || st?.sessionID || "";
  const hero = progress?.live?.find((h) => h.sessionID === id) || null;
  const pct = st?.contextWindow ? Math.min(100, Math.round((st.contextTokens / st.contextWindow) * 100)) : null;
  return {
    key: win ? win.windowID : "ext:" + id, id, windowID: win ? win.windowID : null,
    external: !win && !!live?.external,
    title: live?.title || st?.title || win?.name || "Untitled session",
    project: proj?.label || base(cwd), projectKey: proj?.key || "", cwd,
    model: st?.model || live?.model || "", provider: st?.provider || "",
    activity: live?.state || st?.activity || "", tool: live?.tool ?? st?.tool ?? "", target: live?.target ?? st?.target ?? "",
    hotspot: !!st?.hotspot, unread: !!(live?.unread ?? st?.unread),
    sinceMs: live?.sinceMs || st?.activitySince || 0,
    crew: live?.crew || [], delegating: live?.delegating || 0, crewOut: live?.crewOut || 0,
    targetRel: live?.targetRel || "", compactedMs: live?.compactedMs || 0,
    ctxPct: pct, ctxBand: st?.band || "", turns: st?.turns ?? live?.messages ?? null,
    rank: prog?.rank || "", hero, tint: projectTint(proj?.key || cwd),
  };
}

export function buildRows({ live = [], windows = [], stats = {}, projects = [], progress = null }) {
  const byId = new Map(live.map((s) => [s.id, s]));
  const used = new Set();
  const rows = windows.map((w) => {
    const st = stats[w.windowID];
    const l = st?.sessionID ? byId.get(st.sessionID) : undefined;
    if (l) used.add(l.id);
    return row({ live: l, win: w, st, projects, progress });
  });
  for (const l of live) if (l.external && !l.orphan && !used.has(l.id)) rows.push(row({ live: l, projects, progress }));
  return rows;
}

// 0 permission · 1 error · 2 finished-unread · 3 working (incl. crew out) · 4 quiet
export function attention(r) {
  if (r.activity === "permission") return 0;
  if (r.activity === "error") return 1;
  if (r.crewOut > 0) return 3;
  if (r.activity === "finished" && r.unread) return 2;
  if (r.activity === "thinking" || r.activity === "tool") return 3;
  return 4;
}

export function bandOf(r) {
  const a = attention(r);
  return a <= 2 ? "needs" : a === 3 ? "working" : "quiet";
}

export function sortRows(rows) {
  return [...rows].sort((a, b) =>
    attention(a) - attention(b) || (a.sinceMs || 0) - (b.sinceMs || 0) || String(a.title).localeCompare(String(b.title)));
}

export function density(n) {
  return n <= 1 ? "hero" : n <= 14 ? "standard" : "compact";
}

export function crewView(crew = [], max = 6) {
  const visible = crew.filter((a) => (a.depth || 1) <= 2);
  const shown = visible.slice(0, max);
  return { shown, more: crew.length - shown.length };
}
