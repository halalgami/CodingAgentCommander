import { test } from "node:test";
import assert from "node:assert/strict";
import { landOf, gridOrigins, packDistrict, MAX_LAND, GAP } from "./layout.js";

const D = (over = {}) => ({ land: 4, parks: 0, buildings: [], blocks: [], oldTownCount: 0, ...over });
const many = (n) => Array.from({ length: n }, (_, i) => ({ sessionID: "b" + i }));

// Owner, 2026-10-14: a big repo with little history was a huge empty lawn. The plot
// now follows what stands on it; codebase size only nudges it (+2 at most), cap 12.
test("landOf: sized by contents, nudged by code size, 4..12", () => {
  assert.equal(landOf(D()), 4);
  assert.equal(landOf(D({ buildings: many(30) })), 9); // ceil(sqrt((30 + 4) * 2))
  assert.equal(landOf(D({ land: 24, buildings: many(1) }), 1), 6); // contents say 4, code nudges +2
  assert.equal(landOf(D({ land: 5, buildings: many(1) }), 1), 5);
  assert.equal(landOf(D({ buildings: many(500) })), MAX_LAND);
  assert.ok(landOf(D({ buildings: many(10) }), 6) > landOf(D({ buildings: many(10) }), 0), "live sites take room");
});

test("gridOrigins: an even grid, stable order, plots and their road rings never touch", () => {
  const lands = [6, 4, 9, 5, 7];
  const o = gridOrigins(lands);
  assert.deepEqual(o, gridOrigins(lands));
  const stride = Math.max(...lands) + GAP;
  assert.deepEqual(new Set(o.map((p) => p.gx)).size, 3, "5 plots → 3 columns");
  const boxes = o.map((p, i) => [p.x0 - 1, p.y0 - 1, p.x0 + lands[i], p.y0 + lands[i]]);
  for (let i = 0; i < boxes.length; i++) {
    assert.ok(boxes[i][0] >= o[i].gx * stride - 1 && boxes[i][2] <= (o[i].gx + 1) * stride, "plot leaves its cell");
    for (let j = i + 1; j < boxes.length; j++) {
      const [a, b] = [boxes[i], boxes[j]];
      assert.ok(a[2] < b[0] || b[2] < a[0] || a[3] < b[1] || b[3] < a[1], `plots ${i} and ${j} touch`);
    }
  }
  assert.deepEqual(gridOrigins([]), []);
});

test("packDistrict: deterministic, inside the plot, sites on the back rows, nothing overlaps", () => {
  const d = D({ land: 8, parks: 3, buildings: many(10), blocks: [{ week: "w" }] });
  const at = { x0: 30, y0: 15, land: 8, sites: 3 };
  const a = packDistrict(d, at), b = packDistrict(d, at);
  assert.deepEqual(a, b);
  assert.equal(a.siteCells.length, 3);
  const cells = new Set();
  for (const c of [...a.lots, ...a.siteCells]) {
    assert.ok(c.x >= 30 && c.x < 38 && c.y >= 15 && c.y < 23, "outside the plot");
    const k = `${c.x},${c.y}`;
    assert.ok(!cells.has(k), "overlap at " + k);
    assert.ok(!(c.x >= a.plaza.x && c.x <= a.plaza.x + 1 && c.y >= a.plaza.y && c.y <= a.plaza.y + 1), "on the plaza");
    cells.add(k);
  }
  for (const s of a.siteCells) assert.ok(s.y - 15 <= 1, "site off the back rows");
  assert.equal(a.lots.filter((l) => l.kind === "building").length, 10);
});

test("packDistrict: sites beyond two back rows are counted, not stacked", () => {
  const p = packDistrict(D(), { x0: 0, y0: 0, land: 4, sites: 20 });
  assert.equal(p.siteCells.length + p.siteOverflow, 20);
  assert.ok(p.siteCells.length <= 8);
});

test("packDistrict: a crowded plot counts what does not fit instead of overlapping", () => {
  const p = packDistrict(D({ parks: 5, buildings: many(30) }), { x0: 0, y0: 0, land: 4, sites: 0 });
  assert.equal(p.lots.filter((l) => l.kind !== "park").length + p.overflow, 30);
  assert.ok(p.lots.length <= 16 - 4);
});

test("packDistrict: parks shrink with the plot", () => {
  const p = packDistrict(D({ land: 24, parks: 100 }), { x0: 0, y0: 0, land: 6, sites: 0 });
  assert.ok(p.lots.filter((l) => l.kind === "park").length <= 7);
});

// Minors round 2 (city retune review): a district with roads === 2 painted its
// middle road under lots. The road's tiles are kept free.
test("packDistrict: the middle road's tiles stay free of lots and sites", () => {
  const d = D({ roads: 2, land: 12, buildings: many(80), parks: 10 });
  const p = packDistrict(d, { x0: 0, y0: 0, land: 9, sites: 4 });
  const mid = Math.floor(9 / 2);
  for (let x = 0; x < 9; x += 3) {
    if (x >= p.plaza.x && x <= p.plaza.x + 1 && mid >= p.plaza.y && mid <= p.plaza.y + 1) continue;
    assert.ok(![...p.lots, ...p.siteCells].some((l) => l.x === x && l.y === mid), `lot on the road at ${x},${mid}`);
  }
});
