// The slot ladder: a pure function of a state snapshot plus host-computed
// timers. Evaluated against the SELECTED session only — finish and error are
// per-session fields, so a background session finishing shows bg_finished and a
// bubble while the foreground figure is untouched (spec §5.6).
//
// Liveness comes from a last-output timestamp, never from `status`. `status` is
// "active" from launch until the Stop hook — it means "not yet finished", not
// "doing something" — and SelectSession overwrites it to "active" on selection,
// so deriving `working` or `awaiting` from it makes `awaiting` unreachable for
// any session the user clicks into.
import { DONE_TTL_MS, ERROR_TTL_MS, WORKING_HOLD_MS, BORED_MS } from "./pack.js";

export function selectedSession(state) {
  const list = state && Array.isArray(state.sessions) ? state.sessions : null;
  if (!list || !state.selected) return null;
  return list.find((s) => s && s.windowID === state.selected) ?? null;
}

export function pickSlot(state, facts) {
  const nowMs = facts?.nowMs ?? 0;
  const msSinceInput = facts?.msSinceInput ?? 0;
  const msSinceOutput = facts?.msSinceOutput ?? Infinity;
  const sel = selectedSession(state);

  if (!sel) return msSinceInput > BORED_MS ? "bored" : "idle";

  const errorMs = sel.errorMs ?? 0;
  if (errorMs > 0 && nowMs - errorMs < ERROR_TTL_MS) return "error";

  const finishMs = sel.lastFinishMs ?? 0;
  if (finishMs > 0 && nowMs - finishMs < DONE_TTL_MS) return "done";

  // An unconsumed finish: the run ended and nothing has been printed since.
  // Infinity msSinceOutput (no output ever seen) makes lastOutputMs -Infinity,
  // which is correctly "before the finish".
  const lastOutputMs = nowMs - msSinceOutput;
  if (finishMs > 0 && lastOutputMs <= finishMs) return "awaiting";

  // Reaching here already proves a turn is in flight: an error, a fresh finish
  // and an unconsumed finish were all ruled out above, so output has arrived
  // since the last finish. The only remaining job is a backstop for a finish
  // signal that never comes. An earlier 3s window asked "is output arriving
  // right now", which is a different question and read every quiet gap inside
  // one turn as "stopped".
  if (msSinceOutput < WORKING_HOLD_MS) return "working";
  if (msSinceInput > BORED_MS) return "bored";
  return "idle";
}
