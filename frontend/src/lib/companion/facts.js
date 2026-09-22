// Host-layer glue between the app's live state and the pure decision core.
// Four jobs, all of them pure so they are table-testable without a DOM:
//
//   buildFacts        — assembles the exact `facts` object conditions/slots/mood/
//                       resolver read. Every field is supplied; an unsupplied one
//                       reads as undefined, every comparison against it is false,
//                       and the system collapses to a single constant.
//   pruneFinishes     — windows the finish log, because conditions.js counts the
//                       array length and the host owns the 10-minute window.
//   withLatchedError  — the frontend error latch. reportError already emits
//                       app:error to the deck, so attributing it to the selected
//                       session here needs no new Go state (spec §6.4).
//   createDwell       — the minimum slot dwell. Without it,
//                       working -> done -> awaiting -> working is a slideshow
//                       during exactly the sessions people watch (spec §4.3).
import { ERROR_TTL_MS } from "./pack.js";
import { STREAK_WINDOW_MS } from "./conditions.js";
import { selectedSession } from "./slots.js";

export const SLOT_DWELL_MS = 8000;

// Slots that may cut a dwell short. Everything else is a mood change; these two
// are INFORMATION, and hiding them behind the dwell is worse than the
// transition it costs. Adopting one restarts the dwell, so neither can flicker.
//
// `done` is here because it was otherwise unreachable for most real turns.
// DONE_TTL_MS is 5s and the dwell is 8s, so a turn finishing at t=3s left the
// gate holding `working` until t=8s — by which point the ladder's candidate had
// already moved past `done` to `awaiting`. The celebratory image only ever
// appeared after turns longer than the dwell, which is not what the user
// bought it for. Both halves were individually tested; nothing composed
// dwell.gate(pickSlot(...)) over time, which is exactly what the host does.
export const DWELL_PREEMPT = Object.freeze(["error"]);

export function localParts(nowMs) {
  const d = new Date(nowMs);
  return { hour: d.getHours(), weekday: d.getDay() };
}

export function pruneFinishes(list, nowMs) {
  if (!Array.isArray(list)) return [];
  return list.filter((t) => typeof t === "number" && Number.isFinite(t) && nowMs - t < STREAK_WINDOW_MS);
}

export function buildFacts({
  state, nowMs, msSinceInput, msSinceOutput,
  finishTimes = [], firstRunOfDay = false, freshInstall = false,
  parts = localParts,
} = {}) {
  const sel = selectedSession(state);
  const { hour, weekday } = parts(nowMs);
  return {
    nowMs,
    hour,
    weekday,
    msSinceInput,
    msSinceOutput,
    selectedActiveMs: sel && sel.statusSinceMs > 0 ? nowMs - sel.statusSinceMs : 0,
    finishSeq: state?.finishSeq ?? 0,
    recentFinishMs: pruneFinishes(finishTimes, nowMs),
    firstRunOfDay: firstRunOfDay === true,
    freshInstall: freshInstall === true,
  };
}

// Returns a state whose SELECTED session carries the latched error. Shallow
// clones rather than mutating: the input is a reactive proxy owned by the store
// and by Go, and writing into it would make the latch permanent.
export function withLatchedError(state, errorMs, nowMs) {
  if (!(errorMs > 0) || nowMs - errorMs >= ERROR_TTL_MS) return state;
  const sel = selectedSession(state);
  if (!sel) return state;
  // A newer per-session error wins. Still return a fresh top-level object
  // rather than the literal `state` reference: this branch has already
  // decided the latch is live and eligible, so a caller diffing by reference
  // to know "did the latch pass just run" must see a new object even when
  // its conclusion was "no change" — only the truly-nothing-to-do exits
  // above (non-positive/expired errorMs, no selection) return the input as-is.
  if ((sel.errorMs ?? 0) >= errorMs) return { ...state };
  return {
    ...state,
    sessions: state.sessions.map((s) =>
      s && s.windowID === sel.windowID ? { ...s, errorMs } : s),
  };
}

export function createDwell({ minMs = SLOT_DWELL_MS, preempt = DWELL_PREEMPT } = {}) {
  let held = null;
  let since = -Infinity;
  return {
    gate(slot, nowMs) {
      if (held === null || slot === held) {
        if (held === null) { held = slot; since = nowMs; }
        return held;
      }
      // Adopt the CURRENT candidate when the window expires, not the one that
      // first differed — a slot that flickered at t+1s must not land at t+8s.
      if (preempt.includes(slot) || nowMs - since >= minMs) {
        held = slot;
        since = nowMs;
      }
      return held;
    },
    held() { return held; },
  };
}
