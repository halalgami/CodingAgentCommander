import { test } from "node:test";
import assert from "node:assert/strict";
import { placeSigns, signK, SIGN, CHIP } from "./signs.js";

const hit = (a, b) => !(a.x + a.w <= b.x || b.x + b.w <= a.x || a.y + a.h <= b.y || b.y + b.h <= a.y);
const anchors = (n, dx = 30) => Array.from({ length: n }, (_, i) => ({ key: "s" + i, x: 400 + i * dx, y: 300 }));

test("signK: callouts shrink with the zoom, never below 0.7 or above 1", () => {
  assert.equal(signK(2), 1); assert.equal(signK(0.85), 0.85); assert.equal(signK(0.2), 0.7);
});

// Owner, 2026-10-14: callouts covered the buildings and each other.
test("placeSigns: crowded towers get callouts that never overlap and stay on screen", () => {
  const out = placeSigns(anchors(5), 1, 1200, 800, []);
  const boxes = [...out.values()];
  assert.equal(boxes.length, 5);
  for (const b of boxes) assert.ok(b.x >= 0 && b.y >= 0 && b.x + b.w <= 1200 && b.y + b.h <= 800, "off screen");
  const placed = boxes.filter((b) => !b.forced);
  for (let i = 0; i < placed.length; i++) for (let j = i + 1; j < placed.length; j++) assert.ok(!hit(placed[i], placed[j]), "overlap");
  assert.ok(placed.some((b) => b.mode === "full"));
});

test("placeSigns: zoomed out below 0.6 every callout is a one-line chip", () => {
  const out = placeSigns(anchors(3, 200), 0.5, 1200, 800, []);
  for (const b of out.values()) assert.equal(b.mode, "chip");
  assert.equal([...out.values()][0].h, CHIP.h * signK(0.5));
});

test("placeSigns: callouts avoid obstacles (plates)", () => {
  const plate = { x: 460, y: 200, w: 300, h: 200 }; // right of the tower, where the first spot would be
  const out = placeSigns([{ key: "a", x: 450, y: 300 }], 1, 1200, 800, [plate]);
  assert.equal(out.get("a").forced, false);
  assert.ok(!hit(out.get("a"), plate));
});

test("placeSigns: no room anywhere → a forced chip right above the tower, never dropped", () => {
  const out = placeSigns([{ key: "a", x: 50, y: 30 }], 1, 120, 60, []);
  const b = out.get("a");
  assert.equal(b.mode, "chip"); assert.equal(b.forced, true);
  assert.ok(Math.abs(b.x + b.w / 2 - 50) < 1e-9);
});

test("placeSigns: full callouts have the documented size", () => {
  const b = placeSigns([{ key: "a", x: 600, y: 400 }], 1, 1200, 800, []).get("a");
  assert.deepEqual([b.w, b.h, b.mode], [SIGN.w, SIGN.h, "full"]);
});

// Final review I3: callouts must not cover the neighbouring towers either.
test("placeSigns: callouts avoid tower bodies, their own and their neighbours'", () => {
  const anchors = [0, 1, 2].map((i) => ({ key: "t" + i, x: 500 + i * 32, y: 300, body: { x: 500 + i * 32 - 28, y: 300, w: 56, h: 90 } }));
  const out = placeSigns(anchors, 1, 1200, 800, []);
  for (const b of out.values()) {
    if (b.forced) continue;
    for (const a of anchors) assert.ok(!hit(b, a.body), "sign covers tower " + a.key);
  }
  assert.ok([...out.values()].some((b) => !b.forced));
});

test("towerTop: the anchor rises with the tower's growth", async () => {
  const { towerTop } = await import("./signs.js");
  assert.ok(towerTop(300, 1, 8) < towerTop(300, 1, 0));
  assert.equal(towerTop(300, 2, 0), 300 - 64 * 2);
});

// Minors round 2 (city retune review): forced chips could stack on each other.
test("placeSigns: forced chips are stacked so they never overlap", () => {
  const anchors = [0, 1, 2].map((i) => ({ key: "a" + i, x: 100 + i * 4, y: 200 }));
  const out = placeSigns(anchors, 1, 200, 220, [{ x: 0, y: 0, w: 200, h: 220 }]); // everything blocked
  const boxes = [...out.values()];
  assert.ok(boxes.every((b) => b.forced));
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) assert.ok(!hit(boxes[i], boxes[j]), "forced chips overlap");
});

// Minors round 2 (city retune review): every plate reserved 260 px, so a short
// name kept callouts needlessly far away.
test("plateWidth: follows the label, capped at the plate's max width", async () => {
  const { plateWidth } = await import("./signs.js");
  assert.ok(plateWidth("app", "Outpost") < 140);
  assert.equal(plateWidth("a-very-long-project-name-that-goes-on-and-on", "Settlement"), 260);
});
