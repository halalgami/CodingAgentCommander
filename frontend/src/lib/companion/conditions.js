// Named predicates over the host's `facts` snapshot. No expression language:
// a parser has no authoring-time feedback, fails open into silence, and would
// mean the generator writing strings into a parser we also wrote (spec §5.5).
//
// Two classes, and the difference is the whole point:
//   ambient — evaluate() returns the current level. Gated art joins the base
//             pool while it holds. No flash, no TTL.
//   event   — evaluate() returns true only on a RISING EDGE against `prev`, so
//             it fires once and must go false before it can fire again.
import { CONDITION_CLASS } from "./pack.js";

export const MARATHON_MS = 60 * 60 * 1000;
export const LONG_IDLE_MS = 30 * 60 * 1000;
export const STREAK_N = 3;
export const STREAK_WINDOW_MS = 10 * 60 * 1000; // the host filters recentFinishMs to this
export const CENTURY_N = 100;

// Level predicates. Ambient conditions use these directly; edge conditions use
// them as "is it true now / was it true then".
const LEVEL = {
  lateNight: (f) => f.hour >= 2 && f.hour < 5,
  weekend: (f) => f.weekday === 0 || f.weekday === 6,
  marathon: (f) => f.selectedActiveMs > MARATHON_MS,
  longIdle: (f) => f.msSinceInput > LONG_IDLE_MS,
  freshInstall: (f) => f.freshInstall === true,
  firstRunOfDay: (f) => f.firstRunOfDay === true,
  streak: (f) => (Array.isArray(f.recentFinishMs) ? f.recentFinishMs.length : 0) >= STREAK_N,
  century: () => false, // handled as a crossing below, never as a level
};

export function isEdge(name) {
  return CONDITION_CLASS[name] === "event";
}

export function evaluate(name, facts, prev = null) {
  if (!facts || !(name in CONDITION_CLASS)) return false;

  // `century` is a CROSSING of a multiple of 100, not `finishSeq % 100 === 0`.
  // The modulo form stays true until the next finish and re-fires indefinitely;
  // the bucket form is also immune to a skipped tick (99 -> 201 still fires).
  // With no `prev` there is no baseline, so no crossing can be claimed.
  if (name === "century") {
    if (!prev) return false;
    return bucket(facts.finishSeq) > bucket(prev.finishSeq);
  }

  const level = LEVEL[name];
  if (!isEdge(name)) return level(facts) === true;

  // Rising edge. A missing `prev` counts as "was false", which is correct for
  // launch-scoped flags: firstRunOfDay must fire on the very first tick of the
  // day's first run, and there is no earlier tick to compare against.
  return level(facts) === true && !(prev && level(prev) === true);
}

function bucket(finishSeq) {
  const n = typeof finishSeq === "number" && finishSeq > 0 ? finishSeq : 0;
  return Math.floor(n / CENTURY_N);
}
