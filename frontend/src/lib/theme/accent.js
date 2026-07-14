// Pure accent math. OKLCH -> sRGB conversion (D65), gamut clamping by chroma
// reduction, and derivation of the accent variant set used by tokens.css.
// No DOM access — unit-testable under node:test.

export const DEFAULT_ACCENT = { h: 65, c: 0.145 };

// Accent lightness/chroma are clamped so any user hue stays visible on the
// graphite surfaces and can carry readable on-accent text.
const L_MIN = 0.60, L_MAX = 0.80, L_BASE = 0.74;
const C_MAX = 0.17;

export function oklchToHex(l, c, h) {
  const rad = (h * Math.PI) / 180;
  return oklabToHex(l, c * Math.cos(rad), c * Math.sin(rad));
}

function oklabToHex(L, a, b) {
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.2914855480 * b;
  const l3 = l_ ** 3, m3 = m_ ** 3, s3 = s_ ** 3;
  let r = +4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3;
  let g = -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3;
  let bl = -0.0041960863 * l3 - 0.7034186147 * m3 + 1.7076147010 * s3;
  return "#" + [r, g, bl].map((x) => {
    const srgb = x <= 0.0031308 ? 12.92 * x : 1.055 * Math.pow(Math.max(x, 0), 1 / 2.4) - 0.055;
    return Math.round(Math.min(1, Math.max(0, srgb)) * 255).toString(16).padStart(2, "0");
  }).join("");
}

// True when the un-clamped conversion stays inside sRGB.
function inGamut(l, c, h) {
  const rad = (h * Math.PI) / 180;
  const a = c * Math.cos(rad), b = c * Math.sin(rad);
  const l_ = l + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = l - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = l - 0.0894841775 * a - 1.2914855480 * b;
  const l3 = l_ ** 3, m3 = m_ ** 3, s3 = s_ ** 3;
  const r = +4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3;
  const g = -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3;
  const bl = -0.0041960863 * l3 - 0.7034186147 * m3 + 1.7076147010 * s3;
  return [r, g, bl].every((x) => x >= -0.0001 && x <= 1.0001);
}

// Reduce chroma until the color fits sRGB.
function clampChroma(l, c, h) {
  let lo = 0, hi = c;
  if (inGamut(l, c, h)) return c;
  for (let i = 0; i < 20; i++) {
    const mid = (lo + hi) / 2;
    if (inGamut(l, mid, h)) lo = mid; else hi = mid;
  }
  return lo;
}

export function relativeLuminance(hex) {
  const n = parseInt(hex.slice(1), 16);
  const chan = (v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * chan((n >> 16) & 255) + 0.7152 * chan((n >> 8) & 255) + 0.0722 * chan(n & 255);
}

const ok = (l, c, h) => `oklch(${(l * 100).toFixed(1)}% ${c.toFixed(4)} ${h.toFixed(1)})`;

export function deriveAccent({ h, c }) {
  const hue = ((h % 360) + 360) % 360;
  const cReq = Math.min(Math.max(c, 0.02), C_MAX);
  const l = Math.min(Math.max(L_BASE, L_MIN), L_MAX);
  const cc = clampChroma(l, cReq, hue);
  const accentHex = oklchToHex(l, cc, hue);
  // on-accent text: dark ink on light accents, near-white on dark ones,
  // chosen by WCAG luminance (HSL lightness misjudges cyan vs red).
  const dark = relativeLuminance(accentHex) > 0.30;
  const onL = dark ? 0.20 : 0.97;
  return {
    accent: ok(l, cc, hue),
    accentHover: ok(Math.min(l + 0.06, 0.88), clampChroma(Math.min(l + 0.06, 0.88), cReq, hue), hue),
    accentDim: ok(0.45, clampChroma(0.45, cReq * 0.5, hue), hue),
    accentFaint: ok(0.30, clampChroma(0.30, cReq * 0.25, hue), hue),
    onAccent: ok(onL, clampChroma(onL, 0.03, hue), hue),
    accentHex,
    selectionHex: oklchToHex(0.35, clampChroma(0.35, cReq * 0.4, hue), hue),
  };
}
