// Scene (spec §3–§5, retuned 2026-10-14): districts packed on an even grid, live rows turned into sites on their plot,
// everything projected and sorted back-to-front for the painter.
import { packDistrict, landOf, gridOrigins, cellOrigin, GAP, MAX_LAND } from "./layout.js";
import { toScreen } from "./iso.js";
import { bounds } from "./camera.js";
import { stageOf, floorsOf, cranesOf, workersOf, propOf, poseOf, archetypeOfRow, fireworksOf } from "./sites.js";

function site(row, x, y, district, timeline, now) {
  const { sx, sy } = toScreen(x, y);
  return {
    row, x, y, sx, sy, district,
    archetype: archetypeOfRow(row), stage: stageOf(row.ctxPct), floors: floorsOf(row.turns),
    cranes: cranesOf(row.crew), workers: workersOf(timeline[row.id] || [], now),
    prop: propOf(row), pose: poseOf(row, now), fireworks: fireworksOf(row, now),
  };
}

// siteCountsOf: live sessions per project key (the static city's only live input).
export function siteCountsOf(rows) {
  const m = new Map();
  for (const r of rows) if (r.projectKey) m.set(r.projectKey, (m.get(r.projectKey) || 0) + 1);
  return m;
}

// activeCity keeps only districts with a live session and packs them onto
// plots 1..n in their stored plot order, so a handful of active projects sit
// together instead of scattered across the whole spiral (owner, 2026-10-12).
export function activeCity(city, rows) {
  const live = new Set(rows.map((r) => r.projectKey).filter(Boolean));
  const districts = (city.districts || []).filter((d) => live.has(d.key))
    .sort((a, b) => (a.plot || 0) - (b.plot || 0))
    .map((d, i) => ({ ...d, plot: i + 1 }));
  return { ...city, districts };
}

// buildCity is the static half: districts, lots and their bounds from CityView
// plus how many live sites each district holds (sites stand on the plot, so the
// plot's size depends on that count); individual live ticks never touch it.
export function buildCity(city, siteCounts = new Map()) {
  const list = city.districts || [];
  const lands = list.map((d) => landOf(d, siteCounts.get(d.key) || 0));
  const origins = gridOrigins(lands);
  const stride = lands.length ? Math.max(...lands) + GAP : MAX_LAND + GAP;
  const cols = Math.max(1, Math.ceil(Math.sqrt(lands.length)));
  const districts = [];
  const items = [];
  const pts = [];
  list.forEach((d, i) => {
    const pack = packDistrict(d, { ...origins[i], land: lands[i], sites: siteCounts.get(d.key) || 0 });
    districts.push({ d, pack });
    for (const lot of pack.lots) {
      const { sx, sy } = toScreen(lot.x, lot.y);
      const ref = lot.kind === "building" ? d.buildings[lot.index] : lot.kind === "block" ? d.blocks[lot.index] : { district: d.key };
      items.push({ kind: lot.kind, x: lot.x, y: lot.y, sx, sy, ref: { ...ref, ruins: d.ruins } });
    }
    const x0 = pack.x0 - 1, y0 = pack.y0 - 1, x1 = pack.x0 + pack.land, y1 = pack.y0 + pack.land;
    pts.push(toScreen(x0, y0), toScreen(x1, y1), toScreen(x1, y0), toScreen(x0, y1));
  });
  items.sort(backToFront);
  if (!pts.length) pts.push({ sx: 0, sy: 0 }, { sx: 64, sy: 32 });
  return { districts, items, bounds: bounds(pts), grid: { stride, cols, n: list.length } };
}

const backToFront = (a, b) => a.x + a.y - (b.x + b.y) || a.x - b.x;

// placeSites is the live half: each live row becomes a site on its district's
// back rows, or on a temporary plot in the grid cells after the last district
// when it has no district (history off, new or external folder) or no room left.
export function placeSites(base, rows, timeline, now) {
  const sites = [];
  const byKey = new Map();
  for (const r of rows) {
    const k = r.projectKey || "";
    if (!byKey.has(k)) byKey.set(k, []);
    byKey.get(k).push(r);
  }
  const homeless = [];
  for (const { d, pack } of base.districts) {
    // By key, not attention order: a sibling needing permission must not swap towers.
    const live = [...(byKey.get(d.key) || [])].sort((a, b) => String(a.key).localeCompare(String(b.key)));
    byKey.delete(d.key);
    const cells = pack.siteCells || [];
    live.slice(0, cells.length).forEach((r, i) => sites.push(site(r, cells[i].x, cells[i].y, d.key, timeline, now)));
    if (live.length > cells.length) homeless.push(live.slice(cells.length));
  }
  for (const [, live] of byKey) homeless.push(live);
  // Temporary plots fill the grid cells the districts leave free, on a grid
  // wide enough for both, so history off does not string them out in a line.
  const g = base.grid || { stride: MAX_LAND + GAP, cols: 1, n: 0 };
  const stride = g.n ? g.stride : 6 + GAP;
  const cols = Math.max(g.cols, Math.ceil(Math.sqrt(g.n + homeless.length)));
  const used = new Set(Array.from({ length: g.n }, (_, i) => { const o = cellOrigin(i, g.stride, g.cols); return `${o.x0},${o.y0}`; }));
  const free = [];
  for (let i = 0; free.length < homeless.length; i++) {
    const o = cellOrigin(i, stride, cols);
    if (!used.has(`${o.x0},${o.y0}`)) free.push(o);
  }
  const pts = [];
  homeless.forEach((live, h) => {
    const { x0, y0 } = free[h];
    live.forEach((r, i) => {
      const s = site(r, x0 + (i % 6), y0 + Math.floor(i / 6), "", timeline, now);
      sites.push(s);
      pts.push({ sx: s.sx, sy: s.sy });
    });
  });
  if (!pts.length) return { sites, bounds: base.bounds };
  const b = bounds(pts);
  return { sites, bounds: {
    minX: Math.min(base.bounds.minX, b.minX), maxX: Math.max(base.bounds.maxX, b.maxX),
    minY: Math.min(base.bounds.minY, b.minY), maxY: Math.max(base.bounds.maxY, b.maxY),
  } };
}

// buildScene joins both halves (tests and one-shot callers).
export function buildScene(city, rows, timeline, now) {
  const base = buildCity(city, siteCountsOf(rows));
  const live = placeSites(base, rows, timeline, now);
  const items = [...base.items, ...live.sites.map((s) => ({ kind: "site", x: s.x, y: s.y, sx: s.sx, sy: s.sy, ref: s }))].sort(backToFront);
  return { districts: base.districts, sites: live.sites, items, bounds: live.bounds };
}
