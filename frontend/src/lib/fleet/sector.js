// frontend/src/lib/fleet/sector.js
// Sector Map (phase 7c spec §3): systems, fixed positions, station/dock geometry, orders strip. Pure.
// viewBox 1600 × 900.
import { bandOf } from "../dashboard/rows.js";
import { hashOf } from "../raid/cast.js";
import { callsign, labelSize, labelAll } from "./cic.js";

export const MAP_W = 1600, MAP_H = 900, UNCHARTED = "uncharted";
export const SHIP_W = 48, SHIP_H = 28;
export const CS_FONT = 13, CS_DY = SHIP_H / 2 + 16; // callsign under a waiting ship: font, baseline offset
const NSLOTS = 32, STATION_PX = 96, SHIP_PITCH = 50, DOCK_DY = 58;

export function systemsOf(progress, projects = [], rows = []) {
  const locked = !progress || progress.locked;
  const names = new Map(projects.map((p) => [p.key, p.label]));
  const by = new Map();
  const add = (key, label, hours = 0, rank = "Outpost") => {
    if (!by.has(key)) by.set(key, { key, label: label || key, hours, rank: rank || "Outpost", rows: [] });
    return by.get(key);
  };
  // Only projects with a live session are systems (owner, 2026-10-14, as Al Modon);
  // the ledger supplies their hours and rank, else they are 0 h Outposts.
  const known = new Map(locked ? [] : (progress.projects || []).map((p) => [p.key, p]));
  for (const r of rows) {
    const p = known.get(r.projectKey);
    const s = !r.projectKey ? add(UNCHARTED, "Uncharted")
      : add(r.projectKey, p?.label || names.get(r.projectKey) || r.project, p?.hours || 0, p?.rank);
    s.rows.push(r);
  }
  // Past 32 slots, systems with a waiting session are kept first; offMapOf counts the rest.
  const waiting = (s) => s.rows.some((r) => bandOf(r) === "needs");
  const all = [...by.values()].sort((a, b) => a.key.localeCompare(b.key));
  if (all.length <= NSLOTS) return all;
  const keep = new Set([...all.filter(waiting), ...all.filter((s) => !waiting(s))].slice(0, NSLOTS));
  return all.filter((s) => keep.has(s));
}

// offMapOf: live systems that did not fit on the map's 32 slots.
export function offMapOf(rows) {
  const keys = new Set(rows.map((r) => r.projectKey || UNCHARTED));
  return Math.max(0, keys.size - NSLOTS);
}

// 32 fixed points: hex grid 8 × 4 with a fixed per-slot jitter (±16) so it reads as a starfield.
const jit = (i, axis) => (hashOf(`slot:${i}:${axis}`) % 33) - 16;
export const SLOTS = Array.from({ length: NSLOTS }, (_, i) => {
  const row = Math.floor(i / 8), col = i % 8;
  return { x: 100 + col * 190 + (row % 2 ? 95 : 0) + jit(i, "x"), y: 130 + row * 210 + jit(i, "y") };
});

// Each key's home slot is hash(key) % 32; a taken slot passes to the next free one.
// Keys go in sorted order so the result depends only on the set of keys.
export function placeSystems(systems) {
  const out = new Map(), used = new Set();
  for (const key of systems.map((s) => s.key).sort()) {
    if (used.size >= NSLOTS) break;
    let i = hashOf("sys:" + key) % NSLOTS;
    while (used.has(i)) i = (i + 1) % NSLOTS;
    used.add(i);
    out.set(key, { ...SLOTS[i], slot: i });
  }
  return out;
}

const STATIONS = { Outpost: ["station-1", 0.6], Settlement: ["station-1", 0.8], Station: ["station-2", 1], Citadel: ["station-3", 1.15], Capital: ["station-3", 1.3] };
export function stationOf(rank) {
  const [art, scale] = STATIONS[rank] || STATIONS.Outpost;
  return { art, scale, halo: rank === "Capital" };
}

const ORDER = { needs: 0, working: 1, quiet: 2 };
const age = (r) => r.sinceMs || Infinity; // smaller = older; unknown sorts last
export function dockOf(rows, max = 4) {
  const sorted = [...rows].sort((a, b) => ORDER[bandOf(a)] - ORDER[bandOf(b)] || age(a) - age(b) || String(a.key).localeCompare(String(b.key)));
  return { ships: sorted.slice(0, max), more: Math.max(0, sorted.length - max) };
}

export function geomOf(pos, rank, dock) {
  const st = stationOf(rank);
  const size = STATION_PX * st.scale, sy = pos.y - 10;
  const n = dock.ships.length, y = pos.y + DOCK_DY;
  const ships = dock.ships.map((row, i) => ({ row, x: pos.x - ((n - 1) * SHIP_PITCH) / 2 + i * SHIP_PITCH, y }));
  const moreAt = n && dock.more ? { x: ships[n - 1].x + SHIP_W / 2 + 4, y: y + 5 } : null;
  const right = moreAt ? moreAt.x + 30 : n ? ships[n - 1].x + SHIP_W / 2 + 4 : 0;
  return {
    station: { ...st, x: pos.x, y: sy, size, box: [pos.x - size / 2, sy - size / 2, pos.x + size / 2, sy + size / 2] },
    ships, moreAt,
    dockBox: n ? [ships[0].x - SHIP_W / 2 - 4, y - SHIP_H / 2 - 4, right, y + SHIP_H / 2 + 4] : null,
  };
}

export const ordersStrip = (rows) => rows.filter((r) => bandOf(r) === "needs").sort((a, b) => age(a) - age(b) || String(a.key).localeCompare(String(b.key)));

export function chipText(row, now) {
  const what = row.activity === "error" ? "hull breach" : row.activity === "finished" ? "mission complete"
    : `awaiting ${Math.max(0, Math.floor((now - (row.sinceMs || now)) / 60000))}m`;
  return `${callsign(row)} · ${row.project || "?"} · ${what}`;
}

export const fmtHours = (h) => (h < 10 ? h.toFixed(1) : String(Math.round(h)));

export function headerOf(systems, rows) {
  const h = { awaiting: 0, engaged: 0, docked: 0 };
  for (const r of rows) {
    const b = bandOf(r);
    if (b === "needs") h.awaiting++;
    else if (b === "working") h.engaged++;
    else h.docked++;
  }
  return { ...h, hours: fmtHours(systems.reduce((s, x) => s + (x.hours || 0), 0)), systems: systems.length };
}

export const subOf = (s) => (s.key === UNCHARTED ? "sessions outside any project" : `${fmtHours(s.hours)} h invested · ${s.rank}`);
const inMap = (b) => b[0] >= 8 && b[1] >= 8 && b[2] <= MAP_W - 8 && b[3] <= MAP_H - 8;

// Positions, station/dock geometry and collision-free system labels for the map.
export function layoutSystems(systems, font = 16) {
  const pos = placeSystems(systems);
  const placed = systems.filter((s) => pos.has(s.key)).map((s) => {
    const p = pos.get(s.key);
    const dock = dockOf(s.rows);
    return { sys: s, key: s.key, row: s, x: p.x, y: p.y, more: dock.more, g: geomOf(p, s.rank, dock) };
  });
  // a waiting ship's callsign hangs below the dock and is wider than the ship
  const csBox = (d) => {
    const w = 7.6 * (CS_FONT / 12) * callsign(d.row).length;
    return [d.x - w / 2 - 2, d.y + CS_DY - CS_FONT - 2, d.x + w / 2 + 2, d.y + CS_DY + 5];
  };
  // the station's obstacle is its selection ring (0.7 × size), which also covers
  // the Capital's halo (0.62) — both are wider than the image itself
  const ring = (st) => [st.x - st.size * 0.7, st.y - st.size * 0.7, st.x + st.size * 0.7, st.y + st.size * 0.7];
  const obstacles = placed.flatMap((p) => [ring(p.g.station), ...(p.g.dockBox ? [p.g.dockBox] : []),
    ...p.g.ships.filter((d) => bandOf(d.row) === "needs").map(csBox)]);
  const labels = labelAll(placed, (s) => labelSize([s.label, subOf(s)], font), (s) => labelSize([s.label], font), obstacles, { gap: 72, within: inMap });
  return { placed, labels };
}
