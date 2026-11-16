import { test } from "node:test";
import assert from "node:assert/strict";
import { stackBars, punchcard, calendar, treemap, burnGeom } from "./geom.js";

const close = (a, b, eps = 1e-6) => assert.ok(Math.abs(a - b) <= eps, `${a} ≉ ${b}`);

test("stacked bars: day heights sum to the day total, tallest day fills h", () => {
  const rows = [{ key: "d1", split: "a", value: 30 }, { key: "d1", split: "b", value: 10 }, { key: "d2", split: "a", value: 20 }];
  const { bars, max } = stackBars(rows, { w: 200, h: 100, pad: 0 });
  assert.equal(max, 40);
  close(bars.filter((b) => b.key === "d1").reduce((s, b) => s + b.h, 0), 100);
  close(bars.find((b) => b.key === "d2").h, 50);
  for (const b of bars) assert.ok(b.y + b.h <= 100.0001);
});
test("punchcard: 168 cells, zero → r 0, area ∝ value", () => {
  const cells = punchcard([{ key: "3-9", value: 4 }, { key: "0-0", value: 1 }], { w: 480, h: 140 });
  assert.equal(cells.length, 168);
  const big = cells.find((c) => c.wd === 3 && c.hr === 9), small = cells.find((c) => c.wd === 0 && c.hr === 0);
  assert.equal(cells.find((c) => c.wd === 1 && c.hr === 1).r, 0);
  close((big.r / small.r) ** 2, 4);
});
test("calendar: 13 weeks = 91 cells ending on `to`, level 0 without data", () => {
  const cells = calendar([{ key: "2026-11-12", value: 10 }, { key: "2026-11-11", value: 1 }], { to: "2026-11-12", weeks: 13, cell: 10 });
  assert.equal(cells.length, 91);
  assert.equal(cells.at(-1).day, "2026-11-12");
  assert.equal(cells.at(-1).level, 4);
  assert.equal(cells.find((c) => c.day === "2026-11-01").level, 0);
  assert.ok(cells.find((c) => c.day === "2026-11-11").level > 0);
});
test("treemap: areas ∝ weight and inside bounds", () => {
  const rects = treemap([{ key: "a", weight: 3 }, { key: "b", weight: 1 }], { w: 200, h: 100 });
  const a = rects.find((r) => r.key === "a"), b = rects.find((r) => r.key === "b");
  close(a.w * a.h, 15000, 200);
  close(b.w * b.h, 5000, 200);
  for (const r of rects) { assert.ok(r.x + r.w <= 200.5); assert.ok(r.y + r.h <= 100.5); }
  assert.deepEqual(treemap([], { w: 10, h: 10 }), []);
});

test("calendar: `to` sits in its own weekday row (Mon = 0) for any weekday", () => {
  for (const [to, row] of [["2026-11-16", 0], ["2026-11-18", 2], ["2026-11-15", 6], ["2026-11-12", 3]]) {
    const cells = calendar([], { to, weeks: 13, cell: 10 });
    assert.equal(cells.at(-1).y / 10, row, to);
    assert.equal(cells.at(-8).y, cells.at(-1).y, "a week earlier is the same row");
  }
});
test("stacked bars: days with no data keep their slot when the range is given", () => {
  const { keys, bars } = stackBars([{ key: "2026-11-10", value: 1 }, { key: "2026-11-13", value: 1 }], { w: 400, h: 10, pad: 0, from: "2026-11-10", to: "2026-11-13" });
  assert.deepEqual(keys, ["2026-11-10", "2026-11-11", "2026-11-12", "2026-11-13"]);
  close(bars[1].x, 300);
});

test("burnGeom maps time to x and percent to y (0 % at the bottom)", () => {
  const g = burnGeom([{ t: 0, u: 0 }, { t: 100, u: 100 }], 0, 100, 200, 50);
  assert.equal(g.path, "M0,50L200,0");
  assert.equal(g.forecast, null);
});

test("burnGeom drops points outside the range; empty → empty path", () => {
  assert.equal(burnGeom([{ t: -5, u: 10 }, { t: 50, u: 50 }, { t: 150, u: 90 }], 0, 100, 100, 100).path, "M50,50");
  assert.equal(burnGeom([], 0, 100, 100, 100).path, "");
});

test("burnGeom forecast runs from the last point to 100 %, clipped at the right edge", () => {
  const g = burnGeom([{ t: 0, u: 0 }, { t: 50, u: 25 }], 0, 100, 100, 100, 200);
  assert.deepEqual(g.forecast, { x1: 50, y1: 75, x2: 100, y2: 50 });
  const in_ = burnGeom([{ t: 0, u: 0 }, { t: 50, u: 50 }], 0, 100, 100, 100, 100);
  assert.deepEqual(in_.forecast, { x1: 50, y1: 50, x2: 100, y2: 0 });
});

test("miniTower: min height at 0, full at max, monotone, floors follow height", async () => {
  const { miniTower } = await import("./geom.js");
  assert.deepEqual(miniTower(0, 100), { h: 12, floors: 1, alpha: 0.35 });
  const top = miniTower(100, 100);
  assert.equal(top.h, 120); assert.equal(top.floors, 10); assert.equal(top.alpha, 1);
  assert.ok(miniTower(30, 100).h < miniTower(60, 100).h);
  assert.deepEqual(miniTower(5, 0), { h: 12, floors: 1, alpha: 0.35 });
});
