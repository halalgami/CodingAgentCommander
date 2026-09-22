// Shared vocabulary and timing constants for the holo-room companion. This is
// the root module of the decision core: data only, no logic, no imports, so
// every other module can depend on it without a cycle. Everything exported here
// is frozen because it is shared across modules and a stray mutation would be
// invisible until art stopped showing.

// Foreground slots. This list is the pack format's public API (spec §5.1) —
// what every image is named for and the one artifact that cannot change once
// art exists. Order is the slot ladder's priority order, highest last-resort
// first is NOT implied; see slots.js.
export const SLOTS = Object.freeze(["idle", "working", "done", "awaiting", "error", "bored"]);

// Background slots: generic, reused for every non-selected session.
export const BG_SLOTS = Object.freeze(["bg_idle", "bg_running", "bg_finished", "bg_error"]);

// Go's sessionRec.Status only ever holds "active" or "finished"; "idle" and
// "error" are room-side states. Shipping this as data rather than prose is
// deliberate: the "active" -> "bg_running" rename is exactly what gets
// implemented wrong from a table.
export const BG_FOR = Object.freeze({
  idle: "bg_idle",
  active: "bg_running",
  finished: "bg_finished",
  error: "bg_error",
});

// Two condition classes, because one mechanism suited neither (spec §5.5):
//   ambient — a level. Gated art simply joins the base pool while the condition
//             holds. No TTL, no cooldown, no flash. lateNight art IS the night's art.
//   event   — a rising edge. Fires once, shows for EGG_TTL_MS, and must go false
//             before it can fire again.
export const CONDITION_CLASS = Object.freeze({
  lateNight: "ambient",
  weekend: "ambient",
  marathon: "ambient",
  longIdle: "ambient",
  freshInstall: "ambient",
  firstRunOfDay: "event",
  century: "event",
  streak: "event",
});

// Which conditions can actually be true while a given slot is showing. Without
// this, project B spends money generating art that can never display: the first
// draft's own example paired `lateNight` with the `idle` slot at 3am, which
// `bored` outranks after five minutes of no typing.
//
// Reasoning per exclusion:
//   idle     — longIdle (>30 min no input) is impossible: bored outranks idle at 5 min.
//   working  — marathon lives here only; a finish resets statusSinceMs.
//   done/awaiting — the only slots reachable from a finish, so century/streak land here.
//   error/bored   — reachable at any hour with any input gap, but no finish-scoped
//                   or activity-scoped condition applies.
//
// firstRunOfDay reaches EVERY slot, not just idle: it is a greeting, and the
// first launch of a day does not stay idle — a user who opens Commander and
// immediately launches a session should still get it. Restricting it to
// `idle` would silently clear the condition once the ladder moved past idle,
// and the art would never display: the exact failure mode this matrix exists
// to prevent.
//
// Kept in sync with packCondSlots/packConditions/packSlots/packMoods in
// companionpack.go (inverted there: keyed by condition, not by slot) — the Go
// test TestPackVocabularySyncedWithJS in companionpack_test.go parses this
// file and fails loudly if the two drift.
export const SLOT_CONDITIONS = Object.freeze({
  idle: Object.freeze(["lateNight", "weekend", "freshInstall", "firstRunOfDay"]),
  working: Object.freeze(["lateNight", "weekend", "marathon", "longIdle", "freshInstall", "firstRunOfDay"]),
  done: Object.freeze(["lateNight", "weekend", "longIdle", "freshInstall", "firstRunOfDay", "century", "streak"]),
  awaiting: Object.freeze(["lateNight", "weekend", "longIdle", "freshInstall", "firstRunOfDay", "century", "streak"]),
  error: Object.freeze(["lateNight", "weekend", "longIdle", "freshInstall", "firstRunOfDay"]),
  bored: Object.freeze(["lateNight", "weekend", "longIdle", "freshInstall", "firstRunOfDay"]),
});

// The resolver tick. Everything time-based below is expressed in ms, not ticks,
// so this rate can change without re-authoring a single pack (spec §5.7).
export const RESOLVE_HZ = 2;
export const TICKS_PER_MINUTE = RESOLVE_HZ * 60;

// Eggs: shown for TTL, then unable to re-fire for the refractory. The refractory
// is short on purpose — a long one dominates duty cycle and flattens every
// rarity value into the same on-screen frequency (spec §5.7).
export const EGG_TTL_MS = 12000;
export const EGG_REFRACTORY_MS = 60000;

// Slot ladder windows (spec §5.6).
// How long the ladder keeps reporting `done` after a finish.
//
// It must be >= SLOT_DWELL_MS (8s, facts.js) or the `done` art is unreachable
// for most real turns. At 5s a turn finishing 3s into a dwell expired before
// the gate could adopt it: the gate held `working` until t=8s and by then the
// candidate had already moved on to `awaiting`. The celebratory image only ever
// appeared after turns longer than the dwell.
//
// Raising this rather than adding `done` to DWELL_PREEMPT is deliberate.
// Preempting shows it instantly but reintroduces the slideshow the dwell exists
// to stop — several quick prompts in a row would flash working/done/working.
// With the windows aligned, `done` appears IMMEDIATELY after any turn longer
// than the dwell (the common case, where the gate is already free) and at the
// dwell boundary after a short one. The finish bubble already gives instant
// feedback, so the art does not need to.
//
// Tested by "a short turn still shows done" in slots.test.js, which composes
// the ladder and the gate over time — neither half catches this alone.
export const DONE_TTL_MS = 8000;
export const ERROR_TTL_MS = 30000;
// How long after the last byte of output the figure still reads as `working`.
//
// It replaced a 3s "is output arriving right now" window, which asked the wrong
// question. Claude Code prints nothing for tens of seconds inside a SINGLE turn — a long tool call, a build,
// a think — and treating a 3s gap as "stopped" made the figure alternate
// working/idle for as long as the user watched. SLOT_DWELL_MS did not save it:
// an 8s dwell turns a flicker into a slideshow at exactly the rate people
// notice. This is the backstop for a finish signal that never arrives, not a
// heartbeat: a real finish is caught earlier by `done`/`awaiting`.
//
// It was 120s for one round, which was too long for the WRONG reason: the
// activity timestamp it reads could be stale, because attaching to a pane
// replayed its existing contents and recorded that as output. Every session
// you clicked looked busy for two minutes. With the replay no longer counted
// (see ATTACH_REPLAY_MS), the timestamp is honest, and the window only has to
// span a quiet gap inside one turn rather than excuse a false reading.
//
// The residual limit is worth naming: a pane produces no output during a long
// tool call, so a build that runs past this window reads as idle until it
// prints again. Erring short is the right direction — claiming a session is
// working when it is not is the failure users actually notice.
export const WORKING_HOLD_MS = 45000;
export const BORED_MS = 300000;

// Mood latch (spec §5.4). MOOD_REFRACTORY_MS is not in the spec text; it is the
// "post-expiry refractory" §5.4 requires, sized so an expired transient cannot
// re-latch within a couple of resolver ticks and oscillate.
export const MOOD_DWELL_MS = 3000;
export const MOOD_MAX_HOLD_MS = 20000;
export const MOOD_REFRACTORY_MS = 5000;

// Memory budget inputs (spec §5.9, §4.6). DEPTH_LEVELS is the number of
// pre-baked blur levels for background figures; MAX_BG_FIGURES caps how many
// render at once.
export const DEPTH_LEVELS = 3;
export const MAX_BG_FIGURES = 4;
export const PACK_BITMAP_BUDGET_MB = 64;

// Post-decode validation. Go owns parsing, ids, existence, extension
// whitelisting and slot/condition names, and returns {pack, warnings[]}; this
// adds only what Go cannot know — what the file actually decoded to, and what
// that costs. One warning list, one authority per fact (spec §3).
//
//   dims: { [variantId]: { w, h } } of decoded intrinsic sizes.
//
// The budget warning is on PEAK WORKING SET, not total pack footprint: the
// transition pair, the prefetch target, one bitmap per distinct declared bg slot
// (§5.8 renders only [0]) and the pre-baked blur copies. Total footprint fires
// on every generated pack and stays silent on the pack that actually OOMs.
export function validateDecoded(pack, dims) {
  const warnings = [];
  const slots = pack && typeof pack === "object" && pack.slots && typeof pack.slots === "object"
    ? pack.slots : null;
  if (!slots) return warnings;

  const canvas = pack.canvas && typeof pack.canvas === "object" ? pack.canvas : null;
  const cw = positive(canvas?.w);
  const ch = positive(canvas?.h);
  const sizeOf = (id) => {
    if (!dims || typeof dims !== "object") return null;
    if (!Object.prototype.hasOwnProperty.call(dims, id)) return null;
    const d = dims[id];
    return d && positive(d.w) && positive(d.h) ? d : null;
  };

  const foreground = [];
  const bgFirst = [];

  for (const [slot, list] of Object.entries(slots)) {
    if (!Array.isArray(list)) continue;
    const isBg = BG_SLOTS.includes(slot);
    if (!isBg && !SLOTS.includes(slot)) continue; // unknown slot: Go's warning
    list.forEach((v, index) => {
      if (!v || typeof v.id !== "string" || v.id === "") return;
      const decoded = sizeOf(v.id);
      if (decoded && cw && ch && (decoded.w !== cw || decoded.h !== ch)) {
        warnings.push(
          `${slot}/${v.id}: manifest canvas is ${cw}x${ch} but the file decoded ` +
          `${decoded.w}x${decoded.h}; intrinsic size wins for layout`
        );
      }
      const bytes = bitmapBytes(v, decoded, cw, ch);
      if (isBg) { if (index === 0) bgFirst.push(bytes); }
      else foreground.push(bytes);
    });
  }

  const topThree = foreground.sort((a, b) => b - a).slice(0, 3);
  const largestBg = bgFirst.length > 0 ? Math.max(...bgFirst) : 0;
  const peak =
    sum(topThree) +                 // 2 transition + 1 prefetch
    sum(bgFirst) +                  // one per distinct declared bg slot
    DEPTH_LEVELS * largestBg;       // pre-baked blur copies, budgeted at full size

  const mib = peak / (1024 * 1024);
  if (mib > PACK_BITMAP_BUDGET_MB) {
    warnings.push(
      `peak working set ~${mib.toFixed(1)} MiB exceeds the ${PACK_BITMAP_BUDGET_MB} MiB ` +
      `bitmap budget; the furthest-back background figures will be dropped`
    );
  }
  return warnings;
}

function positive(n) {
  return typeof n === "number" && Number.isFinite(n) && n > 0 ? n : 0;
}

function sum(list) {
  return list.reduce((a, b) => a + b, 0);
}

// A sprite strip costs frames x frame area (spec §5.9); the still `file` is the
// same bitmap re-used, so it is not counted twice.
function bitmapBytes(variant, decoded, cw, ch) {
  const w = decoded ? decoded.w : cw;
  const h = decoded ? decoded.h : ch;
  const frames = positive(variant.anim?.frames) > 1 ? variant.anim.frames : 1;
  return w * h * 4 * frames;
}
