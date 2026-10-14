// Seeded identity sigils (spec S4): a 5×5 mirror-symmetric glyph and a hue
// from the session id. An agent keeps its lead's hue with its own pattern.
export function hashStr(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

export function sigil(id, leadId) {
  const h = hashStr(id || "");
  const hue = hashStr(leadId ?? id ?? "") % 360;
  const bits = Math.imul(h ^ (h >>> 15), 0x2c1b3c6d) >>> 0;
  const cells = new Array(25).fill(false);
  for (let r = 0; r < 5; r++) {
    for (let c = 0; c < 3; c++) {
      const on = ((bits >>> (r * 3 + c)) & 1) === 1;
      cells[r * 5 + c] = on;
      cells[r * 5 + (4 - c)] = on;
    }
  }
  if (!cells.some(Boolean)) cells[12] = true;
  return { hue, cells };
}

export function projectTint(key) {
  return `hsl(${hashStr(key || "") % 360} 55% 50%)`;
}
