// Al Asturlab state + loaders. Every number comes from the Go stats engine;
// nothing here aggregates ledger data.
import { Stats, StatsOverview, StatsModels, ExportStats } from "../../../wailsjs/go/main/App.js";
import { rangeFor, addDays } from "./range.js";

export const ast = $state({
  range: "7d", custom: { from: "", to: "" },
  unit: "tokens", split: "model",
  view: null, // null | { project, label } | { model }
  locked: false, loading: false, error: "",
  overview: { cur: {}, prev: {} }, spend: [], projects: [], punch: [], calendar: [], tools: [], models: [], providers: [],
  pricedAt: "", stale: false,
});

export function currentRange(now = new Date()) {
  const id = ast.range === "custom" && ast.custom.from && ast.custom.to ? ast.custom : ast.range === "custom" ? "7d" : ast.range;
  return rangeFor(id, now);
}

const run = async (q) => { try { const r = await Stats(q); ast.pricedAt = r.pricedAt; ast.stale = r.stale; return r.rows || []; } catch { return []; } };

// Queries for a range and an optional filter (project/model drill-in).
export function queries(r, filter = {}) {
  const base = { from: r.from, to: r.to, ...filter };
  const weeks = r.days > 91 ? 52 : 13;
  return {
    spend: { ...base, groupBy: "day", split: ast.split, metric: ast.unit },
    projects: { ...base, groupBy: "project", metric: "tokens" },
    punch: { ...base, groupBy: "weekday-hour", metric: "turns" },
    calendar: { ...filter, from: addDays(r.to, -(weeks * 7 - 1)), to: r.to, groupBy: "day", metric: "tokens" },
    tools: { ...base, groupBy: "tool", metric: "turns" },
    providers: { ...base, groupBy: "provider", metric: "tokens" },
  };
}

let seq = 0;
export async function load(filter = {}) {
  const my = ++seq;
  const r = currentRange();
  ast.loading = true;
  try {
    const ov = await StatsOverview(r.from, r.to);
    if (my !== seq) return;
    ast.locked = !!ov?.locked;
    if (ast.locked) return;
    ast.overview = ov.overview;
    const q = queries(r, filter);
    const [spend, projects, punch, calendar, tools, models, providers] = await Promise.all([
      run(q.spend), run(q.projects), run(q.punch), run(q.calendar), run(q.tools),
      StatsModels(r.from, r.to, filter.project || "").catch(() => []),
      run(q.providers),
    ]);
    if (my !== seq) return;
    Object.assign(ast, { spend, projects, punch, calendar, tools, models: filter.model ? models.filter((m) => m.model === filter.model) : models, providers });
  } finally {
    if (my === seq) ast.loading = false;
  }
}

export async function exportData(kind, format, q = {}) {
  const r = currentRange();
  try { return await ExportStats(kind, format, r.from, r.to, q); } catch (e) { return ""; }
}

export const filterOf = (view) => (view?.project ? { project: view.project } : view?.model ? { model: view.model } : {});
export const refresh = () => load(filterOf(ast.view));
