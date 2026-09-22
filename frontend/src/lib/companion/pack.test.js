import { test } from "node:test";
import assert from "node:assert/strict";
import {
  SLOTS, BG_SLOTS, BG_FOR, CONDITION_CLASS, SLOT_CONDITIONS,
  RESOLVE_HZ, TICKS_PER_MINUTE, EGG_TTL_MS, EGG_REFRACTORY_MS,
  DONE_TTL_MS, ERROR_TTL_MS, WORKING_HOLD_MS, BORED_MS,
  MOOD_DWELL_MS, MOOD_MAX_HOLD_MS, MOOD_REFRACTORY_MS,
  DEPTH_LEVELS, MAX_BG_FIGURES, PACK_BITMAP_BUDGET_MB,
  validateDecoded,
} from "./pack.js";
import { SLOT_DWELL_MS } from "./facts.js";
import { ECHO_WINDOW_MS, RESIZE_REPAINT_MS, ATTACH_REPLAY_MS } from "../termbus.js";

test("the foreground slot vocabulary is frozen and complete", () => {
  assert.deepEqual(SLOTS, ["idle", "working", "done", "awaiting", "error", "bored"]);
  assert.ok(Object.isFrozen(SLOTS), "SLOTS must be frozen: it is shared data");
});

test("background slots are the four generic ones", () => {
  assert.deepEqual(BG_SLOTS, ["bg_idle", "bg_running", "bg_finished", "bg_error"]);
  assert.ok(Object.isFrozen(BG_SLOTS));
});

test("BG_FOR maps the Go status enum, including the active -> bg_running rename", () => {
  // Go's sessionRec.Status only ever holds "active" or "finished" (spec §6.2);
  // "idle" and "error" are room-side states. The active -> bg_running rename is
  // the exact thing that gets implemented wrong from a prose table.
  assert.equal(BG_FOR.active, "bg_running");
  assert.equal(BG_FOR.finished, "bg_finished");
  assert.equal(BG_FOR.idle, "bg_idle");
  assert.equal(BG_FOR.error, "bg_error");
  assert.deepEqual(Object.keys(BG_FOR).sort(), ["active", "error", "finished", "idle"]);
  for (const v of Object.values(BG_FOR)) assert.ok(BG_SLOTS.includes(v), `${v} not a bg slot`);
});

test("every condition carries a class, and the classes are the spec's", () => {
  assert.deepEqual(CONDITION_CLASS, {
    lateNight: "ambient",
    weekend: "ambient",
    marathon: "ambient",
    longIdle: "ambient",
    freshInstall: "ambient",
    firstRunOfDay: "event",
    century: "event",
    streak: "event",
  });
  assert.ok(Object.isFrozen(CONDITION_CLASS));
});

test("SLOT_CONDITIONS is keyed by every foreground slot and names only known conditions", () => {
  assert.deepEqual(Object.keys(SLOT_CONDITIONS).sort(), [...SLOTS].sort());
  for (const [slot, names] of Object.entries(SLOT_CONDITIONS)) {
    assert.ok(Array.isArray(names), `${slot} must map to an array`);
    for (const n of names) {
      assert.ok(n in CONDITION_CLASS, `${slot} names unknown condition ${n}`);
    }
  }
});

test("the matrix encodes the three unreachability findings from the review", () => {
  // longIdle (> 30 min without input) cannot coexist with `idle`, because
  // `bored` outranks `idle` after 5 minutes without input (spec §5.5, §5.6).
  assert.ok(!SLOT_CONDITIONS.idle.includes("longIdle"));
  // marathon requires a session that has been "active" for > 60 min, and a
  // finish resets statusSinceMs, so it only ever reaches `working`.
  const withMarathon = Object.entries(SLOT_CONDITIONS)
    .filter(([, ns]) => ns.includes("marathon")).map(([s]) => s);
  assert.deepEqual(withMarathon, ["working"]);
  // century and streak are finish events: only done/awaiting can show them.
  for (const name of ["century", "streak"]) {
    const slots = Object.entries(SLOT_CONDITIONS)
      .filter(([, ns]) => ns.includes(name)).map(([s]) => s).sort();
    assert.deepEqual(slots, ["awaiting", "done"], `${name} reaches the wrong slots`);
  }
});

test("timing constants match the spec and derive consistently", () => {
  assert.equal(RESOLVE_HZ, 2);
  assert.equal(TICKS_PER_MINUTE, RESOLVE_HZ * 60);
  assert.equal(EGG_TTL_MS, 12000);
  assert.equal(EGG_REFRACTORY_MS, 60000);
  assert.equal(DONE_TTL_MS, 8000);
  assert.equal(ERROR_TTL_MS, 30000);
  // Long on purpose: it is a backstop for a missing finish signal, not a
  // liveness heartbeat. See the note in pack.js.
  assert.equal(WORKING_HOLD_MS, 45000);
  assert.equal(BORED_MS, 300000);
  assert.equal(MOOD_DWELL_MS, 3000);
  assert.equal(MOOD_MAX_HOLD_MS, 20000);
  assert.ok(MOOD_REFRACTORY_MS > 0 && MOOD_REFRACTORY_MS < MOOD_MAX_HOLD_MS);
  assert.equal(DEPTH_LEVELS, 3);
  assert.equal(MAX_BG_FIGURES, 4);
  assert.equal(PACK_BITMAP_BUDGET_MB, 64);
});

const canvasPack = (canvas, slots) => ({ schema: 1, canvas, alpha: true, slots });
const SMALL = { w: 832, h: 1216, scale: 2 };   // 3.86 MiB decoded
const HUGE = { w: 2048, h: 3072, scale: 2 };   // 24.00 MiB decoded

test("a decoded size that disagrees with the manifest canvas warns", () => {
  const pack = canvasPack(SMALL, { idle: [{ id: "idle-a", file: "a.png" }] });
  const warnings = validateDecoded(pack, { "idle-a": { w: 800, h: 1200 } });
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /idle-a/);
  assert.match(warnings[0], /832x1216/);
  assert.match(warnings[0], /800x1200/);
  assert.match(warnings[0], /intrinsic size wins/);
});

test("a matching or undecoded variant produces no size warning", () => {
  const pack = canvasPack(SMALL, { idle: [{ id: "idle-a", file: "a.png" }] });
  assert.deepEqual(validateDecoded(pack, { "idle-a": { w: 832, h: 1216 } }), []);
  assert.deepEqual(validateDecoded(pack, {}), [], "an undecoded variant is not an error");
});

test("total pack footprint is NOT what warns", () => {
  // 20 variants x 3.86 MiB = 77 MiB total, over the 64 MiB budget — but the
  // peak working set is three bitmaps, so this pack is fine and must be silent.
  // A total-footprint warning fires on every pack the generator produces.
  const idle = Array.from({ length: 20 }, (_, i) => ({ id: `idle-${i}`, file: `${i}.png` }));
  assert.deepEqual(validateDecoded(canvasPack(SMALL, { idle }), {}), []);
});

test("peak working set warns on the pack that actually OOMs", () => {
  // 3 foreground (2 transition + 1 prefetch) + 2 bg slots + 3 depth levels,
  // all at 24 MiB = 192 MiB.
  const pack = canvasPack(HUGE, {
    idle: [{ id: "a" }, { id: "b" }, { id: "c" }],
    bg_idle: [{ id: "bgi" }],
    bg_running: [{ id: "bgr" }],
  });
  const warnings = validateDecoded(pack, {});
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /peak working set/);
  assert.match(warnings[0], /64 MiB/);
});

test("the same layout at authored resolution stays under budget", () => {
  // 8 bitmaps x 3.86 MiB = 30.9 MiB.
  const pack = canvasPack(SMALL, {
    idle: [{ id: "a" }, { id: "b" }, { id: "c" }],
    bg_idle: [{ id: "bgi" }],
    bg_running: [{ id: "bgr" }],
  });
  assert.deepEqual(validateDecoded(pack, {}), []);
});

test("only the first variant of each bg slot counts, per §5.8", () => {
  // Background figures never go through the resolver: each renders
  // pack.slots[BG_FOR[status]][0]. Extra bg variants are legal but never
  // decoded, so they must not inflate the peak.
  const many = Array.from({ length: 30 }, (_, i) => ({ id: `bg-${i}` }));
  const pack = canvasPack(SMALL, { idle: [{ id: "a" }], bg_idle: many });
  assert.deepEqual(validateDecoded(pack, {}), []);
});

test("a sprite strip is budgeted as frames x frame area", () => {
  const pack = canvasPack(SMALL, {
    idle: [{ id: "loop", file: "a.png", anim: { strip: "s.png", frames: 24, fps: 8 } }],
  });
  const warnings = validateDecoded(pack, {});
  assert.equal(warnings.length, 1, "24 frames x 3.86 MiB = 92.6 MiB should warn");
  assert.match(warnings[0], /peak working set/);
});

test("decoded dimensions, not the manifest, drive the budget", () => {
  // The manifest claims a small canvas; the files are actually huge.
  const pack = canvasPack(SMALL, { idle: [{ id: "a" }, { id: "b" }, { id: "c" }] });
  const dims = { a: { w: 4096, h: 4096 }, b: { w: 4096, h: 4096 }, c: { w: 4096, h: 4096 } };
  const warnings = validateDecoded(pack, dims);
  assert.equal(warnings.filter((w) => /peak working set/.test(w)).length, 1);
  assert.equal(warnings.filter((w) => /intrinsic size wins/.test(w)).length, 3);
});

test("validateDecoded does not duplicate Go's schema checks", () => {
  // Unknown slot names, unknown conditions and bad rarity are Go's warnings
  // (spec §3, §5.10). Reporting them here too gives two authorities and one
  // divergence.
  const pack = canvasPack(SMALL, {
    idle: [{ id: "a", when: "totallyMadeUp", rarity: 0 }],
    not_a_slot: [{ id: "x" }],
    bored: [{ id: "b", when: "century" }],   // incompatible pairing, Go's call
  });
  assert.deepEqual(validateDecoded(pack, {}), []);
});

test("validateDecoded never throws on a malformed pack", () => {
  assert.deepEqual(validateDecoded(null, {}), []);
  assert.deepEqual(validateDecoded({}, {}), []);
  assert.deepEqual(validateDecoded({ slots: { idle: "nope" } }, {}), []);
  assert.deepEqual(validateDecoded({ slots: { idle: [null, { id: "a" }] } }, undefined), []);
});

// The relationship is load-bearing, not incidental: if DONE_TTL_MS drops below
// the dwell, the `done` art becomes unreachable for every turn shorter than the
// dwell and nothing else in the suite notices.
test("the done window is at least as long as the slot dwell", () => {
  assert.ok(DONE_TTL_MS >= SLOT_DWELL_MS,
    `DONE_TTL_MS (${DONE_TTL_MS}) < SLOT_DWELL_MS (${SLOT_DWELL_MS}): done expires before the dwell can adopt it`);
});

// The suppression windows live in termbus, which is a NEUTRAL seam that
// survives the public export and therefore cannot import anything from here.
// The relationship between them and the liveness window is a companion
// concern, so the cross-check lives on this side of the boundary.
//
// If a suppression window ever grew to a material fraction of WORKING_HOLD_MS,
// hiding echo or a repaint would also blind the ladder to real activity.
test("the activity-suppression windows stay small against the liveness window", () => {
  assert.ok(ECHO_WINDOW_MS * 10 < WORKING_HOLD_MS,
    `echo window ${ECHO_WINDOW_MS}ms is a material fraction of ${WORKING_HOLD_MS}ms`);
  assert.ok(RESIZE_REPAINT_MS * 5 < WORKING_HOLD_MS,
    `resize window ${RESIZE_REPAINT_MS}ms is a material fraction of ${WORKING_HOLD_MS}ms`);
  assert.ok(ATTACH_REPLAY_MS * 5 < WORKING_HOLD_MS,
    `attach window ${ATTACH_REPLAY_MS}ms is a material fraction of ${WORKING_HOLD_MS}ms`);
});
