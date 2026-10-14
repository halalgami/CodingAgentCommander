import { test } from "node:test";
import assert from "node:assert/strict";
import { fit, zoomAt, pan, worldToView, viewToWorld } from "./camera.js";

test("fit centres the bounds and clamps scale", () => {
  const c = fit({ minX: 0, minY: 0, maxX: 1000, maxY: 500 }, 1000, 500);
  assert.equal(c.x, 500); assert.equal(c.y, 250); assert.ok(c.scale > 0.85 && c.scale <= 0.9);
  assert.equal(fit({ minX: 0, minY: 0, maxX: 1, maxY: 1 }, 1000, 1000).scale, 2.5);
});

// Review Focus 3: clicks after pan/zoom hit the right world point.
test("view ↔ world stays exact after zoom and pan", () => {
  let c = { x: 100, y: 50, scale: 1 };
  c = zoomAt(c, 1.5, 300, 200, 800, 600);
  c = pan(c, 40, -25);
  for (const [vx, vy] of [[0, 0], [300, 200], [799, 599]]) {
    const w = viewToWorld(c, vx, vy, 800, 600);
    const v = worldToView(c, w.wx, w.wy, 800, 600);
    assert.ok(Math.abs(v.vx - vx) < 1e-9 && Math.abs(v.vy - vy) < 1e-9);
  }
  const before = viewToWorld({ x: 100, y: 50, scale: 1 }, 300, 200, 800, 600);
  const after = viewToWorld(zoomAt({ x: 100, y: 50, scale: 1 }, 1.5, 300, 200, 800, 600), 300, 200, 800, 600);
  assert.ok(Math.abs(before.wx - after.wx) < 1e-9, "zoom keeps the point under the cursor");
  assert.equal(zoomAt({ x: 0, y: 0, scale: 2.4 }, 2, 0, 0, 10, 10).scale, 2.5);
});

// Owner, 2026-10-14: zoom needed tuning. Scroll and pinch both zoom (pinch is
// ctrlKey wheel on macOS); the factor is smooth and bounded per event.
test("wheelFactor: scroll up zooms in, down zooms out; pinch is stronger; bounded", async () => {
  const { wheelFactor } = await import("./camera.js");
  assert.ok(wheelFactor({ deltaY: -100 }) > 1 && wheelFactor({ deltaY: 100 }) < 1);
  assert.ok(wheelFactor({ deltaY: -10, ctrlKey: true }) > wheelFactor({ deltaY: -10 }));
  assert.equal(wheelFactor({ deltaY: -100000 }), 2);
  assert.equal(wheelFactor({ deltaY: 100000 }), 0.5);
  assert.equal(wheelFactor({ deltaY: 0 }), 1);
});

test("shouldRefit: never moved, or untouched for 30 s", async () => {
  const { shouldRefit } = await import("./camera.js");
  assert.equal(shouldRefit(null, 1000), true);
  assert.equal(shouldRefit(1000, 20000), false);
  assert.equal(shouldRefit(1000, 31001), true);
});

test("zoom stays within 0.3–2.5", async () => {
  const { zoomAt, MIN_SCALE, MAX_SCALE } = await import("./camera.js");
  assert.deepEqual([MIN_SCALE, MAX_SCALE], [0.3, 2.5]);
  assert.equal(zoomAt({ x: 0, y: 0, scale: 0.31 }, 0.1, 0, 0, 100, 100).scale, 0.3);
});
