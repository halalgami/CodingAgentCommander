// The single output-stage governor on the region's composited mean luminance.
//
// The parameter that matters is DEPTH, not rate. The previous draft capped
// modulation at <=1Hz while swinging alpha 0.55 -> 1.0: a 29% modulation, some
// 20-30x the flicker detection threshold — slow enough to pass the seizure
// criterion and deep enough to be maximally distracting (spec §4.3, defect 7).
//
// It is ONE governor with ONE scalar input on purpose. Three modulators each
// obeying their own cap still compose into an ungoverned region, which is
// exactly how the old design leaked: the cap lived in one module and three
// other modulators bypassed it. There is no second entry point here.
//
// Three stages, in order:
//   1. depth cap    — clamp into a +/- cap band around base. Michelson contrast
//                     between the band edges is exactly `cap`.
//   2. retrigger    — a direction reversal may not happen more often than
//                     minRetriggerMs, so the region cannot buzz at the edges.
//   3. slew         — never move faster than slewPerSec, so even a legal step
//                     inside the band arrives as a glide.

export const MICHELSON_CAP = 0.05;
export const SLEW_PER_SEC = 0.25;
export const MIN_RETRIGGER_MS = 1000;

export function michelson(min, max) {
  const lo = Math.min(min, max);
  const hi = Math.max(min, max);
  const sum = lo + hi;
  if (!(sum > 0)) return 0;
  return (hi - lo) / sum;
}

export function createLimiter({
  base = 1,
  cap = MICHELSON_CAP,
  slewPerSec = SLEW_PER_SEC,
  minRetriggerMs = MIN_RETRIGGER_MS,
} = {}) {
  const lo = base * (1 - cap);
  const hi = base * (1 + cap);

  let out = base;
  let lastMs = null;
  let dir = 0;                 // -1 falling, +1 rising, 0 settled
  let dirSinceMs = -Infinity;
  let seenLo = base;
  let seenHi = base;

  function observe(v) {
    if (v < seenLo) seenLo = v;
    if (v > seenHi) seenHi = v;
  }

  return {
    apply(requested, nowMs, { frozen = false } = {}) {
      const dt = lastMs === null ? 0 : Math.max(0, (nowMs - lastMs) / 1000);
      lastMs = nowMs;

      // Suppression holds the current value. Snapping back to base would make
      // the freeze itself the most visible luminance step in the whole feature.
      if (frozen) { observe(out); return out; }

      let target = Number.isFinite(requested) ? requested : base;
      target = Math.min(hi, Math.max(lo, target));

      const want = target > out ? 1 : target < out ? -1 : 0;
      if (want !== 0 && want !== dir) {
        if (nowMs - dirSinceMs < minRetriggerMs) target = out;
        else { dir = want; dirSinceMs = nowMs; }
      }

      const maxStep = slewPerSec * dt;
      const delta = target - out;
      out = Math.abs(delta) <= maxStep ? target : out + Math.sign(delta) * maxStep;

      observe(out);
      return out;
    },
    band() { return { lo, hi }; },
    observed() { return { lo: seenLo, hi: seenHi, depth: michelson(seenLo, seenHi) }; },
  };
}
