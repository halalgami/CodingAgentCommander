// Mood for the holo-room: adapts reactions.js rather than calling it raw, and
// latches the result so art is not re-picked every tick.
//
// Two jobs, both of which the first draft's "reused, expression only" skipped:
//
// 1. A FULLY DEFAULTED input. reactions.js reads ten fields; any one left
//    undefined makes every comparison false and collapses the whole system to a
//    single constant expression (spec §5.4).
// 2. A latch. reactions.js is edge-detected, so happy/surprised exist for one
//    tick. A steady mood (neutral/relaxed) tracks the raw expression at once; a
//    transient is HELD, and ends only when a different transient displaces it
//    after MOOD_DWELL_MS, or when MOOD_MAX_HOLD_MS expires it — which also arms
//    a per-mood refractory so it cannot immediately re-latch and oscillate.
//    Hence "only neutral and relaxed may be steady-state" is a real property.
//
// The gesture half of the reactions return is discarded: the room has no rig.
import { Reactions } from "./reactions.js";
import { selectedSession } from "./slots.js";
import { ERROR_TTL_MS, MOOD_DWELL_MS, MOOD_MAX_HOLD_MS, MOOD_REFRACTORY_MS } from "./pack.js";

export const MOODS = Object.freeze(["neutral", "relaxed", "happy", "sad", "surprised"]);
export const STEADY_MOODS = Object.freeze(["neutral", "relaxed"]);

const isTransient = (m) => !STEADY_MOODS.includes(m);
const steadyOf = (m) => (STEADY_MOODS.includes(m) ? m : "neutral");

export function reactionInput(state, facts) {
  const sel = selectedSession(state);
  const nowMs = facts?.nowMs ?? 0;
  const msSinceInput = facts?.msSinceInput ?? 0;
  const msSinceOutput = facts?.msSinceOutput ?? Infinity;
  const errorMs = sel?.errorMs ?? 0;
  const finishMs = sel?.lastFinishMs ?? 0;
  return {
    // Quota is deferred in v1: PlanUsage is a live HTTPS call that triggers a
    // keychain prompt on darwin and returns nothing for API-key users, so no
    // background poller exists to set this (spec §6.4). `sad` is therefore
    // unreachable, which mood.test.js asserts on purpose.
    quotaHigh: false,
    running: state?.running ?? 0,
    finished: state?.finished ?? 0,
    awaiting: finishMs > 0 && nowMs - msSinceOutput <= finishMs,
    error: errorMs > 0 && nowMs - errorMs < ERROR_TTL_MS,
    click: null,      // hotspot clicks belong to the overlay, not the room
    rc: false,        // no remote-control signal reaches the room
    greeted: true,    // the room has no greeting gesture to fire
    idleMs: msSinceInput,
    nowMs,
  };
}

export function createMood({ now = Date.now } = {}) {
  // rng is never exercised: pooled gesture picks only happen on a click, and
  // reactionInput never sends one. Injected anyway so nothing here is random.
  const reactions = new Reactions(() => 0);
  const refractoryUntil = Object.create(null);
  let mood = "neutral";
  let since = -Infinity;
  let primed = false;

  const latch = (next, t) => { mood = next; since = t; };
  const canLatch = (m, t) => !(m in refractoryUntil) || t >= refractoryUntil[m];

  return {
    update(state, facts) {
      const t = typeof facts?.nowMs === "number" ? facts.nowMs : now();
      // Call decide() on EVERY tick: it is edge-detected against its own prev,
      // so skipping a tick loses an edge permanently.
      const raw = reactions.decide(reactionInput(state, facts)).expression;

      // First tick only records a baseline. reactions.js reads a null prev as
      // "finished was 0, error was false", so mounting into finished or errored
      // sessions would otherwise flash a transient at startup.
      if (!primed) { primed = true; latch(steadyOf(raw), t); return mood; }

      const held = t - since;

      if (isTransient(mood)) {
        if (held >= MOOD_MAX_HOLD_MS) {
          refractoryUntil[mood] = t + MOOD_REFRACTORY_MS;
          latch(steadyOf(raw), t);
          return mood;
        }
        if (held < MOOD_DWELL_MS) return mood;
        if (isTransient(raw) && raw !== mood && canLatch(raw, t)) latch(raw, t);
        return mood; // a steady raw never cuts a live transient short
      }

      if (raw === mood) return mood;
      if (isTransient(raw) && !canLatch(raw, t)) return mood;
      latch(raw, t);
      return mood;
    },
  };
}
