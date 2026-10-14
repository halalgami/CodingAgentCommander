// Camera: world point (x, y) at the view centre, scaled (spec §5).
export const MIN_SCALE = 0.3, MAX_SCALE = 2.5;
const clamp = (s) => Math.min(MAX_SCALE, Math.max(MIN_SCALE, s));
const REFIT_AFTER_MS = 30_000;

// wheelFactor turns one wheel event into a zoom factor: scroll (mouse or
// two-finger) zooms gently, a trackpad pinch (ctrlKey) more; capped per event.
export function wheelFactor(e) {
  const k = e.ctrlKey ? 0.01 : 0.002;
  return Math.min(2, Math.max(0.5, Math.exp(-(e.deltaY || 0) * k)));
}

// shouldRefit: the view follows the city (refits when it changes) unless the
// user moved the camera in the last 30 s.
export const shouldRefit = (movedAt, now) => movedAt == null || now - movedAt > REFIT_AFTER_MS;

export function bounds(points) {
  const b = { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity };
  for (const p of points) {
    b.minX = Math.min(b.minX, p.sx); b.maxX = Math.max(b.maxX, p.sx);
    b.minY = Math.min(b.minY, p.sy); b.maxY = Math.max(b.maxY, p.sy);
  }
  return b;
}

export function fit(b, vw, vh) {
  const bw = Math.max(1, b.maxX - b.minX), bh = Math.max(1, b.maxY - b.minY);
  return { x: (b.minX + b.maxX) / 2, y: (b.minY + b.maxY) / 2, scale: clamp(Math.min(vw / bw, vh / bh) * 0.9) };
}

export function worldToView(c, wx, wy, vw, vh) {
  return { vx: (wx - c.x) * c.scale + vw / 2, vy: (wy - c.y) * c.scale + vh / 2 };
}

export function viewToWorld(c, vx, vy, vw, vh) {
  return { wx: (vx - vw / 2) / c.scale + c.x, wy: (vy - vh / 2) / c.scale + c.y };
}

// zoomAt keeps the world point under (px, py) fixed.
export function zoomAt(c, factor, px, py, vw, vh) {
  const w = viewToWorld(c, px, py, vw, vh);
  const scale = clamp(c.scale * factor);
  return { x: w.wx - (px - vw / 2) / scale, y: w.wy - (py - vh / 2) / scale, scale };
}

export function pan(c, dx, dy) {
  return { ...c, x: c.x - dx / c.scale, y: c.y - dy / c.scale };
}
