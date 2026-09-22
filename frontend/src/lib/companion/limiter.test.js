import { test } from "node:test";
import assert from "node:assert/strict";
import {
  MICHELSON_CAP, SLEW_PER_SEC, MIN_RETRIGGER_MS, michelson, createLimiter,
} from "./limiter.js";

const NOW = 1_700_000_000_000;
const EPS = 1e-9;

test("the depth cap is 5% Michelson, and the constants are exported", () => {
  assert.equal(MICHELSON_CAP, 0.05);
  assert.ok(SLEW_PER_SEC > 0);
  assert.equal(MIN_RETRIGGER_MS, 1000);
});

test("michelson is (max-min)/(max+min), order-insensitive and safe at zero", () => {
  assert.ok(Math.abs(michelson(0.95, 1.05) - 0.05) < EPS);
  assert.equal(michelson(1, 1), 0);
  assert.equal(michelson(1.05, 0.95), michelson(0.95, 1.05));
  assert.equal(michelson(0, 0), 0);
});

test("the swing the previous draft shipped really is a 29% modulation", () => {
  // Pinned so the number in the spec cannot silently become folklore.
  assert.ok(michelson(0.55, 1.0) > 0.29, "the 0.55->1.0 alpha swing is not 29%");
});

test("a 0.55 <-> 1.0 square wave comes out under the 5% cap", () => {
  const lim = createLimiter({ base: 1 });
  let t = NOW;
  for (let i = 0; i < 240; i++) {          // 2 minutes at the resolver tick
    lim.apply(i % 2 === 0 ? 0.55 : 1.0, t);
    t += 500;
  }
  const { depth } = lim.observed();
  assert.ok(depth <= MICHELSON_CAP + EPS, `observed depth ${depth} exceeds the cap`);
});

test("the cap is a band around base, so output can never leave it", () => {
  const lim = createLimiter({ base: 1 });
  const { lo, hi } = lim.band();
  assert.ok(Math.abs(lo - 0.95) < EPS && Math.abs(hi - 1.05) < EPS);
  let t = NOW;
  for (const req of [-5, 0, 0.2, 1, 3, 100, NaN, Infinity, undefined]) {
    const out = lim.apply(req, (t += 5000));   // long dt so slew never binds
    assert.ok(out >= lo - EPS && out <= hi + EPS, `${req} -> ${out} left the band`);
  }
});

test("the slew limit bounds how fast the region may change", () => {
  const lim = createLimiter({ base: 1, slewPerSec: 0.25 });
  lim.apply(1, NOW);                            // establish the clock
  const out = lim.apply(1.05, NOW + 100);       // 0.1s -> at most 0.025
  assert.ok(Math.abs(out - 1.025) < EPS, `slewed to ${out}`);
});

test("a direction reversal cannot happen more often than MIN_RETRIGGER_MS", () => {
  const lim = createLimiter({ base: 1 });
  lim.apply(1, NOW);
  const up = lim.apply(1.05, NOW + 2000);
  assert.ok(up > 1);
  const blocked = lim.apply(0.95, NOW + 2100);  // reversal 100ms later
  assert.equal(blocked, up, "the region reversed inside the retrigger window");
  const allowed = lim.apply(0.95, NOW + 2000 + MIN_RETRIGGER_MS);
  assert.ok(allowed < up, "the reversal never happened even after the window");
});

test("frozen holds the last output rather than snapping to base", () => {
  // Freezing during pty output and just after a keystroke must not itself be a
  // visible luminance step (spec §4.3).
  const lim = createLimiter({ base: 1 });
  lim.apply(1, NOW);
  const moved = lim.apply(1.05, NOW + 5000);
  assert.ok(moved > 1);
  assert.equal(lim.apply(0.5, NOW + 5500, { frozen: true }), moved);
  assert.equal(lim.apply(0.5, NOW + 9000, { frozen: true }), moved);
});

test("this governor takes exactly one scalar input", () => {
  // The prior draft's failure mode was three modulators each obeying their own
  // cap and composing into an ungoverned region. A single scalar input is what
  // keeps THIS governor from repeating that.
  //
  // Scope, stated honestly: this pins the limiter's own surface, NOT that the
  // rendered region has only one modulator. The component also animates the
  // art's opacity when a variant changes (the fade-through-backdrop that stops
  // A->B ghosting), which is a second, deliberately ungoverned luminance
  // change. It is bounded by how often a variant can change (SLOT_DWELL_MS for
  // slots, EGG_TTL_MS + the per-egg refractory for eggs) rather than by this
  // cap. Claiming otherwise here is what let the original defect in.
  const lim = createLimiter();
  assert.deepEqual(Object.keys(lim).sort(), ["apply", "band", "observed"]);
  assert.equal(lim.apply.length >= 1, true);
});

test("observed() reports the real extremes seen, so a test can assert on them", () => {
  const lim = createLimiter({ base: 1 });
  let t = NOW;
  for (let i = 0; i < 100; i++) lim.apply(i % 2 ? 2 : 0, (t += 1000));
  const { lo, hi, depth } = lim.observed();
  assert.ok(lo >= 0.95 - EPS && hi <= 1.05 + EPS);
  assert.ok(Math.abs(depth - michelson(lo, hi)) < EPS);
});

test("a constant request produces a constant output: no self-oscillation", () => {
  const lim = createLimiter({ base: 1 });
  let t = NOW;
  const outs = new Set();
  for (let i = 0; i < 200; i++) outs.add(lim.apply(1, (t += 500)));
  assert.deepEqual([...outs], [1]);
});

test("a custom base scales the band proportionally", () => {
  const lim = createLimiter({ base: 0.8 });
  const { lo, hi } = lim.band();
  assert.ok(Math.abs(michelson(lo, hi) - MICHELSON_CAP) < EPS);
  assert.ok(lo < 0.8 && hi > 0.8);
});
