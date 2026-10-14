import { test } from "node:test";
import assert from "node:assert/strict";
import { layoutCode } from "./layout.js";
import { paintCodeStatic, hitLot, lotScreen } from "./painter.js";
import { worldToView } from "../city/camera.js";

const noop = () => {};
const fakeCtx = () => ({ fillStyle: "", strokeStyle: "", lineWidth: 1, globalAlpha: 1, clearRect: noop, beginPath: noop, moveTo: noop, lineTo: noop, closePath: noop, fill: noop, stroke: noop, drawImage: noop, fillRect: noop });

const big = () => {
  const files = [];
  for (let i = 0; i < 800; i++) files.push({ path: `p${i % 40}/f${i}.go`, folder: `p${i % 40}`, lang: "Go", code: 50 + (i * 37) % 3000, complexity: i % 90, todos: i % 3, flagged: i % 50 === 0 });
  const slabs = Array.from({ length: 200 }, (_, i) => ({ folder: `p${i % 40}`, files: 5, code: 100 }));
  return { files, slabs, folders: [] };
};

test("static scene of 800 files + 200 slabs builds and paints under 50 ms", () => {
  const v = big();
  const t0 = performance.now();
  const l = layoutCode(v);
  paintCodeStatic(fakeCtx(), l, new Map(), { x: 0, y: 0, scale: 0.5 }, 1200, 800);
  assert.ok(performance.now() - t0 < 50, `took ${performance.now() - t0} ms`);
});

test("hover hits the drawn box above its lot", () => {
  const l = layoutCode({ files: [{ path: "tall.go", folder: "", code: 300, complexity: 500 }], slabs: [], folders: [] });
  const lot = l.lots[0];
  const cam = { x: 0, y: 0, scale: 1 };
  const { sx, sy } = lotScreen(lot);
  const v = worldToView(cam, sx, sy, 800, 600);
  assert.equal(hitLot(l, cam, 800, 600, v.vx, v.vy - 60)?.ref.path, "tall.go");
  assert.equal(hitLot(l, cam, 800, 600, v.vx, v.vy - 400), null);
});

// Heat tints the ground under the buildings (spec §5), so the ground has its
// own layer below the heat and the buildings layer paints no ground.
test("ground and buildings paint on separate layers", async () => {
  const { paintCodeGround } = await import("./painter.js");
  const l = layoutCode({ files: [{ path: "a/x.go", folder: "a", lang: "Go", code: 300, complexity: 20 }], slabs: [], folders: [] });
  const fills = (fn) => { const seen = []; const ctx = { ...fakeCtx(), fill() { seen.push(this.fillStyle); } }; fn(ctx); return seen; };
  const cam = { x: 0, y: 0, scale: 1 };
  const ground = fills((ctx) => paintCodeGround(ctx, l, cam, 800, 600));
  const bldg = fills((ctx) => paintCodeStatic(ctx, l, new Map(), cam, 800, 600));
  assert.equal(ground.length, l.folders.length + 1, "street + one block per folder");
  for (const c of ground) assert.ok(!bldg.includes(c), "building layer repaints ground colour " + c);
});

test("hover finds a slab too", () => {
  const l = layoutCode({ files: [], slabs: [{ folder: "", files: 40, code: 900 }], folders: [] });
  const lot = l.lots[0];
  const cam = { x: 0, y: 0, scale: 1 };
  const { sx, sy } = lotScreen(lot);
  const v = worldToView(cam, sx, sy, 800, 600);
  assert.equal(hitLot(l, cam, 800, 600, v.vx, v.vy - 2)?.kind, "slab");
});
