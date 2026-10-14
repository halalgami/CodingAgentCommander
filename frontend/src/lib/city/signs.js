// Session callouts over Al Modon (retuned 2026-10-14): fixed-size boxes that
// shrink with the zoom, placed with the CIC label placer so they never cover
// each other or the district plates; below 0.6× they are one-line chips.
import { labelAll } from "../fleet/cic.js";
import { siteScale } from "./painter.js";

export const SIGN = { w: 220, h: 66 }; // full callout: project, task, activity, button
export const CHIP = { w: 180, h: 24 }; // one line: project · activity
const CHIP_BELOW = 0.6;

export const signK = (scale) => Math.min(1, Math.max(0.7, scale));

const toBox = (b) => [b.x, b.y, b.x + b.w, b.y + b.h];

// plateWidth: a district plate's width from its text (name + rank at --fs-0,
// ~7 px a character, 30 px of padding and gap), capped at its 260 px max-width.
export const plateWidth = (label, rank) => Math.min(260, 7 * (String(label).length + String(rank).length) + 30);

// towerTop: view y of a site's roof, from its ground point; grows with floors as the sprite does.
export const towerTop = (vy, scale, floors) => vy - 64 * scale * siteScale(floors);

// placeSigns: anchors are tower tops in view pixels ({ key, x, y, body? }), body
// being the tower's own box, which no callout may cover; obstacles
// are { x, y, w, h } boxes. Returns key → { x, y, w, h, mode, forced } (top-left).
// A tower with no free spot still gets a chip right above it (forced), so no
// session ever loses its Open button.
export function placeSigns(anchors, scale, vw, vh, obstacles = []) {
  const k = signK(scale);
  const chip = { w: CHIP.w * k, h: CHIP.h * k };
  const full = scale < CHIP_BELOW ? chip : { w: SIGN.w * k, h: SIGN.h * k };
  const within = ([x0, y0, x1, y1]) => x0 >= 0 && y0 >= 0 && x1 <= vw && y1 <= vh;
  const contacts = anchors.map((a) => ({ key: a.key, x: a.x, y: a.y, row: null }));
  const bodies = anchors.filter((a) => a.body).map((a) => a.body);
  const blocks = [...obstacles, ...bodies].map(toBox);
  // The gap clears the widest tower sideways and the callout's half-height above it.
  const gap = Math.max(14 * k, ...bodies.map((b) => b.w / 2 + 6 * k), full.h / 3.2 + 4 * k);
  const placed = labelAll(contacts, () => full, () => chip, blocks, { gap, within });
  const out = new Map();
  const forced = [];
  for (const a of anchors) {
    const p = placed.get(a.key);
    if (p) {
      const mode = p.mode === "full" && full !== chip ? "full" : "chip";
      const size = mode === "full" ? full : chip;
      out.set(a.key, { x: p.box[0], y: p.box[1], w: size.w, h: size.h, mode, forced: false });
    } else {
      // No free spot: a chip right above the tower, stacked upwards past any
      // chip already forced there so two never sit on each other.
      let y = a.y - chip.h - 4 * k;
      const x = a.x - chip.w / 2;
      const clash = (yy) => forced.some((f) => !(x + chip.w <= f.x || f.x + f.w <= x || yy + chip.h <= f.y || f.y + f.h <= yy));
      for (let i = 0; i < 12 && clash(y); i++) y -= chip.h + 2 * k;
      const b = { x, y, w: chip.w, h: chip.h, mode: "chip", forced: true };
      forced.push(b);
      out.set(a.key, b);
    }
  }
  return out;
}
