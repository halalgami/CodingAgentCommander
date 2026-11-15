// Last-7-days usage for the world layers (spec §6): Al Modon economy view,
// Raid pouch, Sector fuel, Fleet glow. Every number comes from Go's Stats; this
// only reshapes rows into lookups. Loaded on demand, at most every 10 s.
import { Stats } from "../../../wailsjs/go/main/App.js";
import { dayKey, addDays } from "./range.js";

export const world = $state({ at: 0, from: "", to: "", projects: {}, providers: {}, models: {}, pricedAt: "", stale: false });

let seq = 0;
export async function ensureWorld(nowMs = Date.now(), force = false) {
  if (!force && world.at && nowMs - world.at < 10_000) return;
  const my = ++seq;
  world.at = nowMs;
  const to = dayKey(new Date(nowMs)), from = addDays(to, -6);
  const q = (groupBy, metric) => Stats({ from, to, groupBy, metric }).catch(() => ({ rows: [] }));
  const [pt, pc, vc, vt, mt] = await Promise.all([q("project", "tokens"), q("project", "cost"), q("provider", "cost"), q("provider", "tokens"), q("model", "tokens")]);
  if (my !== seq) return;
  const projects = {};
  for (const r of pt.rows || []) projects[r.key] = { tokens: r.value, cost: 0, unpriced: 0 };
  for (const r of pc.rows || []) (projects[r.key] ||= { tokens: 0, cost: 0, unpriced: 0 }), (projects[r.key].cost = r.value), (projects[r.key].unpriced = r.unpriced || 0);
  const providers = {};
  for (const r of vt.rows || []) providers[r.key] = { tokens: r.value, cost: 0 };
  for (const r of vc.rows || []) (providers[r.key] ||= { tokens: 0, cost: 0 }).cost = r.value;
  const total = (mt.rows || []).reduce((a, r) => a + r.value, 0);
  const models = {};
  for (const r of mt.rows || []) models[r.key] = { tokens: r.value, share: total > 0 ? r.value / total : 0 };
  Object.assign(world, { from, to, projects, providers, models, pricedAt: pc.pricedAt || "", stale: !!pc.stale });
}

export const maxOf = (rec, k) => Math.max(0, ...Object.values(rec).map((x) => x[k] || 0));
