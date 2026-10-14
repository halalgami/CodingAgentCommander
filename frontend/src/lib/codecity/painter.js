// CodeCity painting (spec §5). Static: ground, folder blocks, file boxes,
// slabs, hotspot scaffolds, ruins. Heat: its own cached layer. Live: cranes,
// workers, foreman, beacons. No text is drawn — the DOM overlay has it all.
import { toScreen, TW } from "../city/iso.js";
import { worldToView } from "../city/camera.js";
import { box, sprite } from "../city/painter.js";
import { boxHeight, colorOf, heatColor } from "./look.js";

const GROUND = "#d6dccb", STREET = "#9aa0a6", DEPTH = ["#cfd8b8", "#c6d0ae", "#bdc8a4", "#b4c09a", "#abb890"];

export const lotScreen = (lot) => toScreen(lot.x + lot.s / 2, lot.y + lot.s / 2);

function quad(ctx, cam, vw, vh, x, y, w, h, color) {
  const pts = [[x, y], [x + w, y], [x + w, y + h], [x, y + h]].map(([a, b]) => {
    const { sx, sy } = toScreen(a, b);
    return worldToView(cam, sx, sy, vw, vh);
  });
  ctx.fillStyle = color;
  ctx.beginPath(); ctx.moveTo(pts[0].vx, pts[0].vy);
  for (const p of pts.slice(1)) ctx.lineTo(p.vx, p.vy);
  ctx.closePath(); ctx.fill();
}

// lotRect: the view rectangle a lot's box covers (drawing and hover share it).
function lotRect(lot, cam, vw, vh) {
  const { sx, sy } = lotScreen(lot);
  const v = worldToView(cam, sx, sy, vw, vh);
  const w = TW * 0.8 * lot.s * cam.scale;
  const h = (lot.kind === "slab" ? 6 : lot.ref.ghost || lot.ref.new ? 3 : boxHeight(lot.ref.complexity)) * cam.scale;
  return { v, w, h, x: v.vx - w / 2, y: v.vy - h, rh: h + w / 2 };
}

const backToFront = (a, b) => a.x + a.y - (b.x + b.y) || a.x - b.x;

// Ground (streets, folder blocks) has its own layer so the heat tint can sit
// between it and the buildings (spec §5).
export function paintCodeGround(ctx, layout, cam, vw, vh) {
  ctx.clearRect(0, 0, vw, vh);
  quad(ctx, cam, vw, vh, -1, -1, layout.side + 2, layout.side + 2, STREET);
  for (const f of layout.folders) quad(ctx, cam, vw, vh, f.x, f.y, f.w, f.h, f.depth === 0 ? GROUND : DEPTH[(f.depth - 1) % DEPTH.length]);
}

export function paintCodeStatic(ctx, layout, sprites, cam, vw, vh) {
  ctx.clearRect(0, 0, vw, vh);
  for (const lot of [...layout.lots].sort(backToFront)) {
    const r = lotRect(lot, cam, vw, vh);
    if (lot.kind === "slab") { box(ctx, r.v.vx, r.v.vy, r.w, r.h, "#b8b2a6"); continue; }
    const f = lot.ref;
    if (f.ghost) { sprite(ctx, sprites, "ruins-0", r.v.vx, r.v.vy, cam.scale * lot.s * 0.6, "#9b9187", 10); continue; }
    if (f.new) { box(ctx, r.v.vx, r.v.vy, r.w, r.h, "#c9b48c"); continue; }
    box(ctx, r.v.vx, r.v.vy, r.w, r.h, colorOf(f));
    if (f.test) { ctx.fillStyle = "rgba(90,170,90,0.85)"; ctx.fillRect(r.v.vx - r.w / 4, r.y - r.w / 8, r.w / 2, r.w / 6); }
    if (f.todos > 0) {
      ctx.strokeStyle = "rgba(60,50,40,0.6)"; ctx.lineWidth = Math.max(1, cam.scale);
      for (let i = 0; i < Math.min(3, f.todos); i++) {
        ctx.beginPath(); ctx.moveTo(r.v.vx - r.w / 4 + i * 4 * cam.scale, r.y + r.h * 0.3);
        ctx.lineTo(r.v.vx - r.w / 4 + i * 4 * cam.scale + 3 * cam.scale, r.y + r.h * 0.5); ctx.stroke();
      }
    }
    if (f.flagged) sprite(ctx, sprites, "scaffold", r.v.vx, r.v.vy, cam.scale * lot.s * 0.8, "rgba(120,120,120,0.5)", r.h / cam.scale);
  }
}

export function paintHeat(ctx, layout, heat, cam, vw, vh) {
  ctx.clearRect(0, 0, vw, vh);
  ctx.globalAlpha = 0.75;
  for (const lot of layout.lots) {
    if (lot.kind !== "file" || !heat.has(lot.ref.path)) continue;
    quad(ctx, cam, vw, vh, lot.x, lot.y, lot.s, lot.s, heatColor(heat.get(lot.ref.path)));
  }
  ctx.globalAlpha = 1;
}

export function paintCodeLive(ctx, layout, cranes, sprites, cam, vw, vh, t, now) {
  ctx.clearRect(0, 0, vw, vh);
  for (const lot of layout.lots) {
    if (lot.kind !== "file") continue;
    const list = cranes.byFile.get(lot.ref.path);
    if (!list) continue;
    const r = lotRect(lot, cam, vw, vh);
    list.forEach((c, i) => {
      const compact = c.row.compactedMs && now - c.row.compactedMs < 60000 ? (now - c.row.compactedMs) / 60000 : null;
      if (compact != null) return; // scaffolding down during compaction
      sprite(ctx, sprites, "crane", r.v.vx + (r.w / 2 + i * 8 * cam.scale), r.v.vy, cam.scale, "#e0b030", 40);
    });
    const waiting = list.some((c) => c.row.activity === "permission");
    if (waiting) sprite(ctx, sprites, "foreman", r.v.vx - r.w / 2 - 6 * cam.scale, r.v.vy + 4 * cam.scale, cam.scale, "#e8833a", 8);
    else for (let i = 0; i < 2; i++) {
      const bob = Math.sin(t / 300 + i + lot.x) * 1.5 * cam.scale;
      sprite(ctx, sprites, "worker-work", r.v.vx + (i ? 6 : -6) * cam.scale, r.v.vy + 4 * cam.scale + bob, cam.scale, "#f2c14e", 6);
    }
  }
}

export function hitLot(layout, cam, vw, vh, px, py) {
  const order = [...layout.lots].sort(backToFront);
  for (let i = order.length - 1; i >= 0; i--) {
    const lot = order[i];
    const r = lotRect(lot, cam, vw, vh);
    if (px >= r.x && px <= r.x + r.w && py >= r.y && py <= r.y + r.rh) return lot;
  }
  return null;
}
