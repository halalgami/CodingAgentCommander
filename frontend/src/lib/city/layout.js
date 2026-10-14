// District placement (spec §5, retuned 2026-10-14): each active district gets a
// plot sized by what stands on it, laid out on an even grid; its live sites are
// built on the plot's back rows, so a session's tower reads as part of its project.
export const MAX_LAND = 12; // tiles per side, largest plot
export const GAP = 3; // tiles between neighbouring plots (roads included)
export const SITE_ROWS = 2; // back rows of a plot that hold live sites

const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

// landOf: room for buildings, weekly blocks, old town, the 2×2 plaza and the live
// sites, at about half density. The codebase size (d.land, from metrics) only
// nudges it, by up to 2 tiles, so a big repo with little history is not a lawn.
export function landOf(d, sites = 0) {
  const contents = (d.buildings || []).length + (d.blocks || []).length + (d.oldTownCount > 0 ? 1 : 0) + 4 + sites;
  const need = Math.ceil(Math.sqrt(contents * 2));
  return clamp(Math.max(need, Math.min(d.land || 4, need + 2)), 4, MAX_LAND);
}

// gridOrigins: plots in plot order on a near-square grid of equal cells, each
// plot centred in its cell, so a handful of districts sit evenly together.
export function gridOrigins(lands) {
  if (!lands.length) return [];
  const stride = Math.max(...lands) + GAP;
  const cols = Math.ceil(Math.sqrt(lands.length));
  return lands.map((land, i) => {
    const gx = i % cols, gy = Math.floor(i / cols);
    const pad = Math.floor((stride - GAP - land) / 2);
    return { gx, gy, x0: gx * stride + pad, y0: gy * stride + pad };
  });
}

// cellOrigin is grid cell i's corner (for temporary plots after the districts).
export function cellOrigin(i, stride, cols) {
  return { x0: (i % cols) * stride, y0: Math.floor(i / cols) * stride };
}

// packDistrict lays a district out deterministically at { x0, y0 } on `land`
// tiles: a 2×2 plaza at the centre, live sites on the back rows (y0, y0 + 1),
// then buildings, weekly blocks and old town on the inner cells, parks last on
// what is left near the edge. What does not fit is counted (`overflow` for
// sessions, `siteOverflow` for live sites), never overlapped.
export function packDistrict(d, { x0 = 0, y0 = 0, land = landOf(d), sites = 0 } = {}) {
  const c = Math.floor(land / 2) - 1;
  const plaza = { x: x0 + c, y: y0 + c };
  const isPlaza = (x, y) => x >= c && x <= c + 1 && y >= c && y <= c + 1;
  const taken = new Set();
  const key = (p) => `${p.x},${p.y}`;

  // A big project's middle road (roads === 2, every third tile) is kept free.
  const roadCells = [];
  if (d.roads === 2) {
    const mid = Math.floor(land / 2);
    for (let x = 0; x < land; x += 3) {
      if (isPlaza(x, mid)) continue;
      taken.add(`${x},${mid}`);
      roadCells.push({ x: x0 + x, y: y0 + mid });
    }
  }

  const siteCells = [];
  for (let y = 0; y < SITE_ROWS && siteCells.length < sites; y++) {
    for (let x = 0; x < land && siteCells.length < sites; x++) {
      if (isPlaza(x, y) || taken.has(`${x},${y}`)) continue;
      taken.add(`${x},${y}`);
      siteCells.push({ x: x0 + x, y: y0 + y });
    }
  }

  const free = [];
  for (let y = 0; y < land; y++) for (let x = 0; x < land; x++) if (!isPlaza(x, y) && !taken.has(`${x},${y}`)) free.push({ x, y });
  const edge = (p) => Math.min(p.x, p.y, land - 1 - p.x, land - 1 - p.y);
  const inner = [...free].sort((a, b) => edge(b) - edge(a) || a.y - b.y || a.x - b.x);
  const outer = [...free].sort((a, b) => edge(a) - edge(b) || a.y - b.y || a.x - b.x);

  const lots = [];
  let overflow = 0;
  const place = (pool, kind, count) => {
    for (let i = 0; i < count; i++) {
      const p = pool.find((q) => !taken.has(key(q)));
      // Only sessions count as "more"; parks and the old-town texture just drop.
      if (!p) { if (kind === "building" || kind === "block") overflow += count - i; return; }
      taken.add(key(p));
      lots.push({ x: x0 + p.x, y: y0 + p.y, kind, index: i });
    }
  };
  place(inner, "building", (d.buildings || []).length);
  place(inner, "block", (d.blocks || []).length);
  place(inner, "oldtown", d.oldTownCount > 0 ? 1 : 0);
  // Parks scale with the plot: metrics sized them for the codebase's land.
  const parks = Math.min(d.parks || 0, Math.round(((d.parks || 0) * land * land) / Math.max(16, (d.land || 4) ** 2)));
  place(outer, "park", parks);
  return { x0, y0, land, plaza, lots, overflow, siteCells, siteOverflow: sites - siteCells.length, roadCells };
}
