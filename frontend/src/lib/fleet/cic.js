// frontend/src/lib/fleet/cic.js
// Fleet CIC (phase 7b spec §3): DRADIS geometry, label placement and the comms log. Pure.
// viewBox 1000 × 1000, centre (500, 500), radius 440; angles in degrees, 0° = up, clockwise.
import { bandOf } from "../dashboard/rows.js";
import { shipName } from "./fleet.js";

export const CX = 500, CY = 500, R = 440;
export const RINGS = { needs: 0.28, working: 0.58, quiet: 0.86 };
const BANDS = ["needs", "working", "quiet"];
const projectOf = (r) => r.projectKey || r.project || "?";

export function polar(angle, r) {
  const a = (angle * Math.PI) / 180;
  return { x: CX + r * Math.sin(a), y: CY - r * Math.cos(a) };
}

export function sectorsOf(rows) {
  const groups = new Map();
  for (const r of rows) {
    const k = projectOf(r);
    const g = groups.get(k) || { key: k, label: r.project || "?", count: 0, needs: 0 };
    g.count++;
    if (bandOf(r) === "needs") g.needs++;
    groups.set(k, g);
  }
  const list = [...groups.values()].sort((a, b) => a.label.localeCompare(b.label) || a.key.localeCompare(b.key));
  const n = list.length;
  if (!n) return [];
  const floor = Math.min(30, 360 / n);
  const spare = 360 - floor * n;
  let at = 0;
  return list.map((g, i) => {
    const span = floor + (spare * g.count) / rows.length;
    const s = { ...g, start: at, end: i === n - 1 ? 360 : at + span };
    at += span;
    return s;
  });
}

export function contactsOf(rows, sectors) {
  const out = [];
  for (const s of sectors) {
    for (const ring of BANDS) {
      const cell = rows.filter((r) => projectOf(r) === s.key && bandOf(r) === ring)
        .sort((a, b) => String(a.key).localeCompare(String(b.key)));
      const inset = Math.min(6, (s.end - s.start) / 4);
      const a0 = s.start + inset, a1 = s.end - inset;
      // When the arc between neighbours is under 22 units (narrow wedge, inner ring),
      // stagger them across the ring band so glyphs and hit areas never pile up.
      const r0 = RINGS[ring] * R;
      const gap = cell.length > 1 ? (r0 * ((a1 - a0) * Math.PI)) / 180 / cell.length : Infinity;
      const STAG = [0, 18, -18];
      cell.forEach((row, i) => {
        const angle = a0 + ((a1 - a0) * (i + 0.5)) / cell.length;
        const r = gap < 22 ? r0 + STAG[i % 3] : r0;
        out.push({ row, key: row.key, ring, angle, ...polar(angle, r) });
      });
    }
  }
  return out;
}

// Label box from its rendered lines (monospace ≈ 7.6 units per char and 15 per line at font 12).
// sectorLabel: a rim sector's name clipped to the chord its arc spans at the
// label radius (hidden below 8 characters), so 15+ sectors never overlap.
const CH = 7.6; // per character at 11 px mono with 0.08em letter-spacing
export function sectorLabel(s) {
  const span = s.end - s.start;
  const chord = span >= 180 ? 2 * (R - 22) : 2 * (R - 22) * Math.sin((span / 2) * Math.PI / 180);
  const max = Math.floor((chord - 8) / CH); // 8: a gap between neighbours
  const full = `SECTOR ${String(s.label).toUpperCase()}`;
  const sub = `${s.count} ship${s.count === 1 ? "" : "s"} · ${s.needs} awaiting`;
  if (max < 8) return { title: "", sub: "", w: 0 };
  const title = full.length > max ? full.slice(0, max - 1) + "…" : full;
  const subFit = sub.length > max ? "" : sub;
  return { title, sub: subFit, w: CH * Math.max(title.length, subFit.length) };
}

// cicLabels: the CIC plot's labels. Encounter boxes bind every label; the plot
// marks (pills, blips, ring and sector names) only push full labels aside.
export function cicLabels(contacts, sectors, encBoxes, fullSize, csSize) {
  return labelAll(contacts, fullSize, csSize, encBoxes, { fullExtra: plotObstacles(contacts, sectors, null) });
}

// plotObstacles: everything a ship label must not cover besides other labels
// and glyphs — the ORDERS pill, crew blips, ring names and rim sector labels.
export function plotObstacles(contacts, sectors, ordersKey) {
  const boxes = [];
  for (const c of contacts) {
    if (c.ring === "needs" && c.key !== ordersKey) boxes.push([c.x - 26, c.y + 15, c.x + 26, c.y + 27]);
    if ((c.row?.crew || []).length) boxes.push([c.x - 30, c.y + 17, c.x + 32, c.y + 28]);
  }
  for (const f of Object.values(RINGS)) boxes.push([CX - 60, CY - f * R - 18, CX + 60, CY - f * R - 2]);
  for (const s of sectors) {
    const { w } = sectorLabel(s);
    if (!w) continue;
    const t = polar(sectors.length === 1 ? 180 : (s.start + s.end) / 2, R - 22);
    boxes.push([t.x - w / 2, t.y - 11, t.x + w / 2, t.y + 18]);
  }
  return boxes;
}

export const labelSize = (lines, font = 12) => ({ w: 7.6 * (font / 12) * Math.max(8, ...lines.map((l) => String(l).length)), h: 15 * (font / 12) * lines.length });

const cands = (G) => [[G, 0, "start"], [-G, 0, "end"], [G, -G, "start"], [G, G, "start"], [-G, -G, "end"], [-G, G, "end"], [0, -1.6 * G, "middle"], [0, 1.6 * G, "middle"]];
const hit = (a, b) => !(a[2] <= b[0] || b[2] <= a[0] || a[3] <= b[1] || b[3] <= a[1]);
const inside = (b) => [[b[0], b[1]], [b[2], b[1]], [b[0], b[3]], [b[2], b[3]]].every(([x, y]) => Math.hypot(x - CX, y - CY) <= R);

// obstacles: extra [x0, y0, x1, y1] boxes labels must avoid (encounter markers). opts.gap: candidate offset;
// opts.within(box): the area a label must stay inside (default: the DRADIS circle).
export function placeLabels(contacts, size, obstacles = [], { gap = 18, within = inside } = {}) {
  const CANDS = cands(gap);
  const glyphs = [...contacts.map((c) => [c.x - 8, c.y - 8, c.x + 8, c.y + 8]), ...obstacles];
  const placed = [];
  const out = new Map();
  for (const c of contacts) {
    const { w, h } = size(c.row, c.key);
    let got = null;
    for (const [dx, dy, anchor] of CANDS) {
      const x0 = anchor === "start" ? c.x + dx : anchor === "end" ? c.x + dx - w : c.x + dx - w / 2;
      const y0 = c.y + dy - h / 2;
      const box = [x0, y0, x0 + w, y0 + h];
      if (!within(box) || placed.some((p) => hit(p, box)) || glyphs.some((g) => hit(g, box))) continue;
      placed.push(box);
      got = { dx, dy, anchor };
      break;
    }
    out.set(c.key, got);
  }
  return out;
}

const boxAt = (c, p, w, h) => {
  const x0 = p.anchor === "start" ? c.x + p.dx : p.anchor === "end" ? c.x + p.dx - w : c.x + p.dx - w / 2;
  return [x0, c.y + p.dy - h / 2, x0 + w, c.y + p.dy + h / 2];
};

// Full labels first; contacts that get none try a callsign-only label that must
// clear every full label too; a contact that fits neither shows its glyph only.
// opts.fullExtra: boxes only FULL labels must avoid; the callsign fallback may
// sit on them, so they never leave a contact with no label at all.
export function labelAll(contacts, fullSize, csSize, obstacles = [], opts = {}) {
  const full = placeLabels(contacts, fullSize, [...obstacles, ...(opts.fullExtra || [])], opts);
  const out = new Map();
  const taken = [];
  for (const c of contacts) {
    const p = full.get(c.key);
    if (!p) continue;
    const { w, h } = fullSize(c.row, c.key);
    const box = boxAt(c, p, w, h);
    taken.push(box);
    out.set(c.key, { ...p, mode: "full", box });
  }
  const rest = contacts.filter((c) => !full.get(c.key));
  const cs = placeLabels(rest, csSize, [...obstacles, ...taken, ...contacts.filter((c) => full.get(c.key)).map((c) => [c.x - 8, c.y - 8, c.x + 8, c.y + 8])], opts);
  for (const c of rest) {
    const p = cs.get(c.key);
    if (!p) { out.set(c.key, null); continue; }
    const { w, h } = csSize(c.row, c.key);
    out.set(c.key, { ...p, mode: "cs", box: boxAt(c, p, w, h) });
  }
  return out;
}

export function ordersOf(contacts, selected) {
  const needs = contacts.filter((c) => c.ring === "needs");
  if (selected && needs.some((c) => c.key === selected)) return selected;
  if (!needs.length) return null;
  const age = (c) => c.row.sinceMs || Infinity;
  return needs.reduce((a, b) => (age(b) < age(a) ? b : a)).key;
}

export const callsign = (row) => shipName(row).replace(/^ACS /, "").toUpperCase();

const ACT = { read: "SENSOR SWEEP", edit: "BEAM", shell: "TORPEDO SALVO", web: "LONG-RANGE SCAN", agent: "LAUNCH", other: "MANOEUVRE" };
// clock: how long ago, coarse ("now", "42s", "3m", "2h"); a mission clock in
// whole minutes read the same on every row (owner, 2026-10-14).
const clock = (ago) => {
  if (!Number.isFinite(ago)) return "";
  const s = Math.max(0, Math.floor(ago / 1000));
  return s < 10 ? "now" : s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m` : `${Math.floor(s / 3600)}h`;
};
const COMMS_MAX = 40; // enough to fill a tall panel; the list scrolls past that

export function commsOf(rows, timeline, now) {
  const items = [];
  for (const row of rows) {
    const cs = callsign(row);
    const evs = timeline[row.id] || [];
    for (const e of evs) {
      if (e.agent) continue;
      if (e.kind === "wait") {
        if (e.label === "start") items.push({ at: e.at, text: `${cs} · AWAITING ORDERS`, tone: "amber" });
        continue;
      }
      if (!ACT[e.kind]) continue;
      items.push({ at: e.at, text: `${cs} · ${ACT[e.kind]}${e.label ? " · " + e.label : ""}`, tone: "green" });
    }
    // A status with no start time cannot be placed in the log: it would be
    // stamped "now" on every tick and never move down.
    if (!row.sinceMs) continue;
    const at = row.sinceMs;
    if (row.activity === "error") items.push({ at, text: `${cs} · HULL BREACH`, tone: "red" });
    if (row.activity === "finished" && row.unread) items.push({ at, text: `${cs} · SECTOR CLEARED`, tone: "cyan" });
    if (row.activity === "permission" && !evs.some((e) => e.kind === "wait" && e.label === "start" && e.at >= at)) {
      items.push({ at, text: `${cs} · AWAITING ORDERS`, tone: "amber" });
    }
  }
  return items.sort((a, b) => b.at - a.at).slice(0, COMMS_MAX).map((i) => ({ ...i, clock: clock(now - i.at) }));
}

export function statusOf(rows) {
  const s = { awaiting: 0, engaged: 0, holding: 0, hostiles: 0 };
  for (const r of rows) {
    const b = bandOf(r);
    if (b === "needs") s.awaiting++;
    else if (b === "working") s.engaged++;
    else s.holding++;
    if (b !== "quiet") s.hostiles++;
  }
  return s;
}
