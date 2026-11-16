// Series → SVG geometry. Pure: no DOM, no aggregation (that is Go's job).
import { squarify } from "../codecity/layout.js";
import { addDays } from "./range.js";

// stackBars: rows {key (day), split, value} → one column per key (in first-seen
// order), segments stacked bottom-up in row order.
// With from/to, every day in the range keeps its column, data or not.
export function stackBars(rows, { w, h, pad = 2, from = "", to = "" }) {
  const seen = new Set(rows.map((r) => r.key));
  if (from && to) for (let d = from, i = 0; d <= to && i < 400; d = addDays(d, 1), i++) seen.add(d);
  const keys = [...seen].sort();
  const tot = new Map(keys.map((k) => [k, 0]));
  for (const r of rows) tot.set(r.key, tot.get(r.key) + Math.max(0, r.value));
  const max = Math.max(0, ...tot.values());
  const colW = keys.length ? w / keys.length : 0;
  const base = new Map(keys.map((k) => [k, h]));
  const bars = [];
  for (const r of rows) {
    const i = keys.indexOf(r.key);
    const bh = max > 0 ? (Math.max(0, r.value) / max) * h : 0;
    const y = base.get(r.key) - bh;
    base.set(r.key, y);
    bars.push({ key: r.key, split: r.split ?? "", value: r.value, unpriced: r.unpriced || 0, x: i * colW + pad / 2, y, w: Math.max(0, colW - pad), h: bh });
  }
  return { bars, max, keys, colW };
}

// punchcard: rows keyed "wd-hr" (Mon = 0) → 168 circles, area ∝ value.
export function punchcard(rows, { w, h }) {
  const v = new Map(rows.map((r) => [r.key, r.value]));
  const max = Math.max(0, ...v.values());
  const cw = w / 24, ch = h / 7, rMax = Math.min(cw, ch) / 2 - 1;
  const cells = [];
  for (let wd = 0; wd < 7; wd++)
    for (let hr = 0; hr < 24; hr++) {
      const value = v.get(`${wd}-${hr}`) || 0;
      cells.push({ wd, hr, value, cx: hr * cw + cw / 2, cy: wd * ch + ch / 2, r: max > 0 ? Math.sqrt(value / max) * rMax : 0 });
    }
  return cells;
}

// calendar: weeks × 7 cells ending on `to` (columns = weeks, rows = Mon..Sun),
// level 0 = no data, 1–4 by quartile of the max.
export function calendar(rows, { to, weeks, cell }) {
  const v = new Map(rows.map((r) => [r.key, r.value]));
  const max = Math.max(0, ...v.values());
  const n = weeks * 7, out = [];
  const end = new Date(to + "T12:00:00");
  const endRow = (end.getDay() + 6) % 7;
  for (let i = n - 1; i >= 0; i--) {
    const day = addDays(to, -i);
    const idx = n - 1 - i + ((endRow + 1) % 7); // first day sits in its own weekday row
    const value = v.get(day) || 0;
    const level = value > 0 && max > 0 ? Math.min(4, Math.ceil((value / max) * 4)) : 0;
    out.push({ day, value, level, x: Math.floor(idx / 7) * cell, y: (idx % 7) * cell });
  }
  return out;
}

// treemap: items {key, weight} → rects via CodeCity's ordered squarify (largest first).
export function treemap(items, { w, h }) {
  const its = items.filter((i) => i.weight > 0).sort((a, b) => b.weight - a.weight);
  if (!its.length) return [];
  return squarify(its, 0, 0, w, h).map(({ it, x, y, w, h }) => ({ key: it.key, x, y, w, h }));
}

// burnGeom: quota samples {t (ms), u (0-100)} → an SVG path over [fromMs, toMs].
// y is 0 % at the bottom; points outside the range are dropped. With forecastMs
// a segment runs from the last point to (forecastMs, 100 %), clipped at w.
export function burnGeom(points, fromMs, toMs, w, h, forecastMs = null) {
  const span = toMs - fromMs;
  if (!(span > 0)) return { path: "", forecast: null };
  const x = (t) => ((t - fromMs) / span) * w;
  const y = (u) => h - (Math.max(0, Math.min(100, u)) / 100) * h;
  const pts = points.filter((p) => p.t >= fromMs && p.t <= toMs).sort((a, b) => a.t - b.t);
  const r = (n) => Math.round(n * 100) / 100;
  const path = pts.map((p, i) => (i ? "L" : "M") + r(x(p.t)) + "," + r(y(p.u))).join("");
  let forecast = null;
  if (forecastMs != null && pts.length) {
    const l = pts[pts.length - 1];
    const x1 = x(l.t), y1 = y(l.u);
    let x2 = x(forecastMs), y2 = 0;
    if (x2 > w && x2 > x1) { y2 = y1 + (0 - y1) * ((w - x1) / (x2 - x1)); x2 = w; }
    forecast = { x1: r(x1), y1: r(y1), x2: r(x2), y2: r(y2) };
  }
  return { path, forecast };
}

// miniTower: the project page's small Al Modon tower (spec §5). Same scale as
// the economy view's pillars: 12–120 px by the project's share of the range's
// biggest project, one floor per 12 px.
export function miniTower(tokens, maxTokens) {
  const s = maxTokens > 0 ? Math.max(0, Math.min(1, (Number(tokens) || 0) / maxTokens)) : 0;
  const h = Math.round(12 + 108 * s);
  return { h, floors: Math.max(1, Math.round(h / 12)), alpha: Math.round((0.35 + 0.65 * s) * 100) / 100 };
}
