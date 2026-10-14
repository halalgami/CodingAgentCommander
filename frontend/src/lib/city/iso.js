// 2:1 isometric projection (spec §5). Sprites are never rotated.
export const TW = 64;
export const TH = 32;

export function toScreen(x, y) {
  return { sx: ((x - y) * TW) / 2, sy: ((x + y) * TH) / 2 };
}

export function toTile(sx, sy) {
  const a = sx / (TW / 2), b = sy / (TH / 2);
  return { x: (a + b) / 2, y: (b - a) / 2 };
}
