// Canvas painting (spec §5). Static: ground, roads, parks, cracks, buildings.
// Blur: one blurred copy for the tilt-shift bands. Live: sites and motion.
// Text is never drawn here — names and titles live in the DOM overlay.
import { SPRITES, spriteKey } from "./manifest.js";
import { toScreen, TW } from "./iso.js";
import { worldToView } from "./camera.js";
import { compacting } from "./sites.js";

export const ARCH_COLOR = { tower: "#7aa7d9", block: "#d9a46e", hall: "#9fc48a" };
const ROAD = ["#8b8f96", "#7d8188", "#6f737a"];
export const YARD = "#c9b48c"; // packed dirt under each live site

export function box(ctx, x, y, w, h, color) {
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.moveTo(x, y - h); ctx.lineTo(x + w / 2, y - h + w / 4); ctx.lineTo(x + w / 2, y + w / 4);
  ctx.lineTo(x, y + w / 2); ctx.lineTo(x - w / 2, y + w / 4); ctx.lineTo(x - w / 2, y - h + w / 4); ctx.closePath();
  ctx.fill();
  ctx.fillStyle = "rgba(0,0,0,0.18)";
  ctx.beginPath(); ctx.moveTo(x, y - h + w / 2); ctx.lineTo(x + w / 2, y - h + w / 4); ctx.lineTo(x + w / 2, y + w / 4); ctx.lineTo(x, y + w / 2); ctx.closePath(); ctx.fill();
}

// spriteRect is the view rectangle a sprite (or its box fallback) covers;
// drawing and hit-testing share it so hover hits what is on screen.
function spriteRect(sprites, key, vx, vy, scale, heightPx = 40) {
  const s = SPRITES[key], img = sprites.get(key);
  if (!s || !img) {
    const w = TW * 0.7 * scale, h = heightPx * scale;
    return { x: vx - w / 2, y: vy - h, w, h: h + w / 2, img: null };
  }
  const w = s.w * scale, h = (img.height / img.width) * w;
  return { x: vx - s.ax * w, y: vy - s.ay * h, w, h, img };
}

export function sprite(ctx, sprites, key, vx, vy, scale, fallbackColor, heightPx = 40) {
  const r = spriteRect(sprites, key, vx, vy, scale, heightPx);
  if (!r.img) { box(ctx, vx, vy, TW * 0.7 * scale, heightPx * scale, fallbackColor); return; }
  ctx.drawImage(r.img, r.x, r.y, r.w, r.h);
}

// siteScale: a live site grows 4% a floor up to 8 floors, so it stays within
// about one tile of its neighbours on the plot.
export const siteScale = (floors) => 1 + Math.min(floors || 0, 8) * 0.04;

// buildingLook picks a static item's sprite key, fallback colour and box height.
function buildingLook(it, sprites) {
  if (it.kind === "park") return { key: sprites.has("park") ? "park" : "trees", color: "#7fb069", h: 8 };
  if (it.kind === "block") return { key: `block-low-${(it.ref.week || "").length % 2}`, color: "#c9b79c", h: 14 };
  if (it.kind === "oldtown") return { key: "oldtown-0", color: "#b8a48a", h: 18 };
  const key = it.ref.ruins ? `ruins-${it.ref.seed % 2}` : spriteKey("building", it.ref.archetype, 3, it.ref.seed);
  const site = spriteKey("site", it.ref.archetype, 3);
  const fallback = sprites.has(site) && !it.ref.ruins ? site : key;
  return { key: sprites.has(key) ? key : fallback, color: ARCH_COLOR[it.ref.archetype] || "#aaa", h: 30 + Math.min(60, it.ref.hours * 6) };
}

// hitItem returns the front-most building, block or old-town lot drawn under
// view point (px, py), or null (Review Focus 3).
export function hitItem(scene, sprites, cam, vw, vh, px, py) {
  for (let i = scene.items.length - 1; i >= 0; i--) {
    const it = scene.items[i];
    if (it.kind === "site" || it.kind === "park") continue;
    const { sx, sy } = toScreen(it.x + 0.5, it.y + 0.5);
    const v = worldToView(cam, sx, sy, vw, vh);
    const look = buildingLook(it, sprites);
    const r = spriteRect(sprites, look.key, v.vx, v.vy, cam.scale, look.h);
    if (px >= r.x && px <= r.x + r.w && py >= r.y && py <= r.y + r.h) return it;
  }
  return null;
}

function tile(ctx, cam, vw, vh, x, y, color) {
  const pts = [[x, y], [x + 1, y], [x + 1, y + 1], [x, y + 1]].map(([a, b]) => {
    const { sx, sy } = toScreen(a, b);
    return worldToView(cam, sx, sy, vw, vh);
  });
  ctx.fillStyle = color;
  ctx.beginPath(); ctx.moveTo(pts[0].vx, pts[0].vy);
  for (const p of pts.slice(1)) ctx.lineTo(p.vx, p.vy);
  ctx.closePath(); ctx.fill();
}

const GROUND = { day: "#cfd8b8", dusk: "#b9b39a", night: "#5d6470" };

export function paintStatic(ctx, scene, sprites, cam, vw, vh, phase) {
  ctx.clearRect(0, 0, vw, vh);
  for (const { d, pack } of scene.districts) {
    for (let y = 0; y < pack.land; y++) for (let x = 0; x < pack.land; x++) {
      tile(ctx, cam, vw, vh, pack.x0 + x, pack.y0 + y, d.ruins ? "#9b9187" : GROUND[phase] || GROUND.day);
    }
    for (let x = -1; x <= pack.land; x++) tile(ctx, cam, vw, vh, pack.x0 + x, pack.y0 - 1, ROAD[d.roads] || ROAD[0]);
    for (const c of pack.siteCells || []) tile(ctx, cam, vw, vh, c.x, c.y, YARD);
    if (d.roads >= 1) for (let y = 0; y < pack.land; y++) tile(ctx, cam, vw, vh, pack.x0 - 1, pack.y0 + y, ROAD[d.roads]);
    for (const c of pack.roadCells || []) tile(ctx, cam, vw, vh, c.x, c.y, ROAD[2]);
    ctx.strokeStyle = "rgba(60,50,40,0.45)"; ctx.lineWidth = Math.max(1, cam.scale);
    for (let i = 0; i < d.cracks; i++) {
      const { sx, sy } = toScreen(pack.x0 + ((i * 7) % pack.land) + 0.3, pack.y0 + ((i * 5) % pack.land) + 0.6);
      const v = worldToView(cam, sx, sy, vw, vh);
      ctx.beginPath(); ctx.moveTo(v.vx, v.vy); ctx.lineTo(v.vx + 6 * cam.scale, v.vy + 2 * cam.scale); ctx.lineTo(v.vx + 9 * cam.scale, v.vy - 1 * cam.scale); ctx.stroke();
    }
  }
  for (const it of scene.items) {
    if (it.kind === "site") continue;
    const { sx, sy } = toScreen(it.x + 0.5, it.y + 0.5);
    const v = worldToView(cam, sx, sy, vw, vh);
    const look = buildingLook(it, sprites);
    sprite(ctx, sprites, look.key, v.vx, v.vy, cam.scale, look.color, look.h);
  }
  // Plaza: a landmark statue for record sessions, a plaque stand for achievements.
  for (const { d, pack } of scene.districts) {
    const { sx, sy } = toScreen(pack.plaza.x + 1, pack.plaza.y + 1);
    const v = worldToView(cam, sx, sy, vw, vh);
    if (d.landmarks.length) sprite(ctx, sprites, "statue", v.vx, v.vy, cam.scale, "#b08d57", 16);
    if (d.plaques.length) sprite(ctx, sprites, "plaque", v.vx + 18 * cam.scale, v.vy + 6 * cam.scale, cam.scale, "#8a6d3b", 6);
  }
}

// The tilt-shift copy: drawn once per static rebuild, shown through the bands.
export function paintBlur(blurCtx, staticCanvas, vw, vh) {
  blurCtx.clearRect(0, 0, vw, vh);
  blurCtx.filter = "blur(3px)";
  blurCtx.drawImage(staticCanvas, 0, 0, vw, vh);
  blurCtx.filter = "none";
}

// windowGlow lights a deterministic scatter of windows on a site at night.
export function windowGlow(ctx, vx, vy, scale, stage, seed) {
  const h = (20 + stage * 14) * scale, w = TW * 0.5 * scale;
  ctx.fillStyle = "rgba(255, 214, 120, 0.85)";
  const rows = Math.max(2, Math.floor(h / (10 * scale)));
  for (let r = 0; r < rows; r++) for (let c = 0; c < 4; c++) {
    if ((seed * 31 + r * 7 + c * 13) % 5 > 2) continue;
    ctx.fillRect(vx - w / 2 + (c + 0.5) * (w / 4) - 1.5 * scale, vy - h + 6 * scale + r * 10 * scale, 3 * scale, 3 * scale);
  }
}

const SKY = { day: null, dusk: "rgba(255,170,90,0.10)", night: "rgba(20,30,70,0.28)" };

export function paintLive(ctx, scene, sprites, cam, vw, vh, t, weather, phase) {
  ctx.clearRect(0, 0, vw, vh);
  for (const s of scene.sites) {
    const { sx, sy } = toScreen(s.x + 0.5, s.y + 0.5);
    const v = worldToView(cam, sx, sy, vw, vh);
    const bob = s.pose === "work" ? Math.sin(t / 300 + s.x) * 1.2 * cam.scale : 0;
    // Compaction (5b): floors compress over 60 s and the cranes come down.
    const compact = compacting(s.row, Date.now());
    const squash = compact != null ? 1 - 0.4 * compact : 1;
    sprite(ctx, sprites, spriteKey("site", s.archetype, s.stage), v.vx, v.vy + bob, cam.scale * siteScale(s.floors) * squash, ARCH_COLOR[s.archetype], 20 + s.stage * 14);
    if (compact == null) for (let i = 0; i < s.cranes.shown; i++) sprite(ctx, sprites, "crane", v.vx + (14 + i * 10) * cam.scale, v.vy, cam.scale, "#e0b030", 40);
    const pose = s.pose === "lunch" ? "worker-lunch" : s.pose === "nap" ? "worker-nap" : "worker-work";
    for (let i = 0; i < Math.max(s.workers, s.pose === "work" ? 1 : 2); i++) {
      const wx = v.vx + (Math.cos(i * 1.7) * 18 + (s.pose === "work" ? Math.sin(t / 500 + i) * 4 : 0)) * cam.scale;
      const wy = v.vy + (6 + Math.sin(i * 2.3) * 6) * cam.scale;
      sprite(ctx, sprites, pose, wx, wy, cam.scale, "#f2c14e", 6);
    }
    if (s.pose === "stop") sprite(ctx, sprites, "foreman", v.vx - 20 * cam.scale, v.vy + 8 * cam.scale, cam.scale, "#e8833a", 8);
    if (s.pose === "fire") {
      sprite(ctx, sprites, "firetruck", v.vx + 26 * cam.scale, v.vy + 10 * cam.scale, cam.scale, "#d64545", 10);
      ctx.fillStyle = "rgba(80,80,80,0.35)";
      for (let i = 0; i < 4; i++) { ctx.beginPath(); ctx.arc(v.vx + Math.sin(t / 400 + i) * 6 * cam.scale, v.vy - (30 + i * 12 + (t / 40) % 12) * cam.scale, (5 + i * 2) * cam.scale, 0, Math.PI * 2); ctx.fill(); }
    }
    if (s.pose === "ribbon") {
      sprite(ctx, sprites, "ribbon", v.vx, v.vy + 12 * cam.scale, cam.scale, "#e05a5a", 4);
      const burst = (t % 2400) / 2400;
      if (s.fireworks && burst < 0.6) for (let i = 0; i < 10; i++) {
        const ang = (i / 10) * Math.PI * 2;
        ctx.fillStyle = `hsla(${(i * 36) % 360} 90% 60% / ${1 - burst / 0.6})`;
        ctx.fillRect(v.vx + Math.cos(ang) * burst * 40 * cam.scale, v.vy - 70 * cam.scale + Math.sin(ang) * burst * 40 * cam.scale, 2 * cam.scale, 2 * cam.scale);
      }
    }
    if (s.prop === "sparks" && s.pose === "work") {
      ctx.fillStyle = "#ffd36e";
      for (let i = 0; i < 5; i++) ctx.fillRect(v.vx + Math.sin(t / 60 + i * 2) * 8 * cam.scale, v.vy - (20 + ((t / 30 + i * 9) % 14)) * cam.scale, 1.5 * cam.scale, 1.5 * cam.scale);
    } else if (s.prop && s.prop !== "drill") {
      const key = s.prop === "inspector" ? "inspector" : s.prop === "mixer" ? "mixer" : "truck";
      sprite(ctx, sprites, key, v.vx + 24 * cam.scale, v.vy + 12 * cam.scale, cam.scale, "#888", 8);
    }
    if (phase === "night" && (s.pose === "work" || s.pose === "ribbon")) windowGlow(ctx, v.vx, v.vy, cam.scale, s.stage, s.x * 7 + s.y);
  }
  if (SKY[phase]) { ctx.fillStyle = SKY[phase]; ctx.fillRect(0, 0, vw, vh); }
  if (weather === "rain") {
    ctx.strokeStyle = "rgba(160,180,220,0.45)"; ctx.lineWidth = 1;
    for (let i = 0; i < 120; i++) {
      const x = (i * 97 + t * 0.2) % vw, y = (i * 53 + t * 0.6) % vh;
      ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x - 3, y + 9); ctx.stroke();
    }
  } else if (weather === "fog") {
    ctx.fillStyle = "rgba(230,230,235,0.22)"; ctx.fillRect(0, 0, vw, vh);
  }
}
