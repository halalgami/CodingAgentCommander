import { test } from "node:test";
import assert from "node:assert/strict";
import { hitItem } from "./painter.js";
import { buildCity } from "./scene.js";
import { worldToView } from "./camera.js";
import { toScreen } from "./iso.js";

const city = { districts: [{ key: "k1", label: "app", plot: 1, land: 4, parks: 0, roads: 0, cracks: 0,
  buildings: [{ sessionID: "tall", archetype: "tower", hours: 10, seed: 1 }], blocks: [], oldTownCount: 0, plaques: [], landmarks: [] }] };

// Review Focus 3: hover hits what is drawn, not the ground tile under it.
test("pointing at a building's body, above its tile, hits that building", () => {
  const base = buildCity(city);
  const b = base.items.find((it) => it.kind === "building");
  for (const scale of [0.5, 1, 2]) {
    const cam = { x: 0, y: 0, scale };
    const { sx, sy } = toScreen(b.x + 0.5, b.y + 0.5);
    const v = worldToView(cam, sx, sy, 800, 600);
    assert.equal(hitItem(base, new Map(), cam, 800, 600, v.vx, v.vy - 50 * scale)?.ref.sessionID, "tall", `scale ${scale}`);
    assert.equal(hitItem(base, new Map(), cam, 800, 600, v.vx, v.vy - 200 * scale), null);
  }
});

// Window glow is drawn in code (the generated overlay kept a black background).
test("night window glow draws small warm lights inside the site, nothing opaque", async () => {
  const { windowGlow } = await import("./painter.js");
  const rects = [];
  const ctx = { fillStyle: "", fillRect(x, y, w, h) { rects.push({ x, y, w, h, style: this.fillStyle }); } };
  windowGlow(ctx, 100, 200, 1, 3, 7);
  assert.ok(rects.length >= 4, "some windows lit");
  for (const r of rects) {
    assert.ok(r.w <= 4 && r.h <= 4, "window-sized");
    assert.ok(r.x > 70 && r.x < 130 && r.y < 200 && r.y > 200 - 80, "inside the building");
    assert.match(r.style, /rgba\(255, ?2\d\d, ?\d+, ?0\.\d+\)/, "warm and translucent");
  }
});

// Live sites stand on a yard beyond the road, not in empty space.
test("each live site's cell on the plot is a construction yard", async () => {
  const { paintStatic, YARD } = await import("./painter.js");
  const fills = [];
  const noop = () => {};
  const ctx = { fillStyle: "", strokeStyle: "", lineWidth: 1, clearRect: noop, beginPath: noop, moveTo: noop, lineTo: noop, closePath: noop, stroke: noop, drawImage: noop,
    fill() { fills.push(this.fillStyle); } };
  const key = city.districts[0].key;
  paintStatic(ctx, buildCity(city, new Map([[key, 3]])), new Map(), { x: 0, y: 0, scale: 1 }, 800, 600, "day");
  assert.equal(fills.filter((f) => f === YARD).length, 3);
});

test("buildings fit inside one tile's width", async () => {
  const { SPRITES } = await import("./manifest.js");
  const { TW } = await import("./iso.js");
  for (const [k, s] of Object.entries(SPRITES)) if (/^(site|building)-/.test(k)) assert.ok(s.w < TW, `${k} is ${s.w} wide`);
});

// 5b compaction pose: while a session compacts, its site's cranes come down.
test("a compacting site draws no cranes", async () => {
  const { paintLive } = await import("./painter.js");
  const { SPRITES } = await import("./manifest.js");
  const sprites = new Map(Object.keys(SPRITES).map((k) => [k, { name: k, width: 64, height: 64 }]));
  const noop = () => {};
  const drawn = [];
  const ctx = { fillStyle: "", strokeStyle: "", lineWidth: 1, clearRect: noop, beginPath: noop, moveTo: noop, lineTo: noop, closePath: noop, stroke: noop, fill: noop, fillRect: noop, arc: noop,
    drawImage(img) { drawn.push(img.name); } };
  const site = (compactedMs) => ({ row: { compactedMs }, x: 0, y: 0, archetype: "tower", stage: 2, floors: 0, cranes: { shown: 2 }, workers: 0, pose: "work", prop: null });
  paintLive(ctx, { sites: [site(0)] }, sprites, { x: 0, y: 0, scale: 1 }, 800, 600, 0, "clear", "day");
  assert.equal(drawn.filter((n) => n === "crane").length, 2);
  drawn.length = 0;
  paintLive(ctx, { sites: [site(Date.now() - 1000)] }, sprites, { x: 0, y: 0, scale: 1 }, 800, 600, 0, "clear", "day");
  assert.equal(drawn.filter((n) => n === "crane").length, 0);
});

// Owner, 2026-10-14: a 25-turn session drew a tower twice a building's width and
// spilled over its neighbours. Growth now tops out at 1.3× (8 floors).
test("siteScale: grows with floors, capped at 1.3×", async () => {
  const { siteScale } = await import("./painter.js");
  assert.equal(siteScale(0), 1);
  assert.ok(siteScale(4) > siteScale(2));
  assert.ok(Math.abs(siteScale(8) - 1.32) < 1e-9);
  assert.equal(siteScale(40), siteScale(8));
});
