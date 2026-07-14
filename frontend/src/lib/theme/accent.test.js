import { test } from "node:test";
import assert from "node:assert/strict";
import { deriveAccent, oklchToHex, relativeLuminance, DEFAULT_ACCENT } from "./accent.js";

test("oklchToHex produces valid hex", () => {
  assert.match(oklchToHex(0.74, 0.145, 65), /^#[0-9a-f]{6}$/);
});

test("white and black extremes", () => {
  assert.equal(oklchToHex(1, 0, 0), "#ffffff");
  assert.equal(oklchToHex(0, 0, 0), "#000000");
});

test("luminance orders correctly", () => {
  assert.ok(relativeLuminance("#ffffff") > 0.9);
  assert.ok(relativeLuminance("#000000") < 0.05);
  assert.ok(relativeLuminance("#ff0000") > relativeLuminance("#220000"));
});

test("deriveAccent clamps wild inputs into safe range", () => {
  // near-black, oversaturated request must still yield a visible accent
  const d = deriveAccent({ h: 250, c: 0.4 });
  assert.match(d.accentHex, /^#[0-9a-f]{6}$/);
  const lum = relativeLuminance(d.accentHex);
  assert.ok(lum > 0.15 && lum < 0.75, `accent luminance ${lum} out of visible range`);
});

test("onAccent contrasts with accent", () => {
  for (const h of [0, 65, 120, 200, 300]) {
    const d = deriveAccent({ h, c: 0.145 });
    const la = relativeLuminance(d.accentHex);
    const lo = relativeLuminance(oklchStringToHexViaDerive(d.onAccent, d));
    // crude contrast ratio check >= 4.5
    const [hi, lo2] = la > lo ? [la, lo] : [lo, la];
    assert.ok((hi + 0.05) / (lo2 + 0.05) >= 4.5, `contrast fail at hue ${h}`);
  }
});

// helper: onAccent is an oklch() string; re-derive its hex through the same converter
function oklchStringToHexViaDerive(s) {
  const m = s.match(/oklch\(([\d.]+)% ([\d.]+) ([\d.]+)\)/);
  return oklchToHex(Number(m[1]) / 100, Number(m[2]), Number(m[3]));
}

test("default accent is the Amber Deck values", () => {
  assert.deepEqual(DEFAULT_ACCENT, { h: 65, c: 0.145 });
});
