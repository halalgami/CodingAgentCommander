import { test } from "node:test";
import assert from "node:assert/strict";
import { pickSlot, selectedSession } from "./slots.js";
import { createDwell, SLOT_DWELL_MS } from "./facts.js";
import { DONE_TTL_MS, ERROR_TTL_MS, WORKING_HOLD_MS, BORED_MS } from "./pack.js";

const NOW = 1_700_000_000_000;

// One selected session plus one background session, both fully populated. Tests
// override single fields so an unset field can never read as undefined.
function makeState(sel = {}, bg = {}) {
  return {
    sessions: [
      { windowID: "w1", name: "alpha", status: "active", statusSinceMs: NOW - 60_000,
        lastFinishMs: 0, errorMs: 0, ...sel },
      { windowID: "w2", name: "beta", status: "active", statusSinceMs: NOW - 60_000,
        lastFinishMs: 0, errorMs: 0, ...bg },
    ],
    selected: "w1",
    running: 2, finished: 0, finishSeq: 0, lastFinished: "",
  };
}

function makeFacts(over = {}) {
  return {
    nowMs: NOW, hour: 14, weekday: 3,
    // Infinity, not a large number: this is termbus's own sentinel for a pane
    // that has never produced output, and it is the only value that is
    // unambiguously "not working" no matter what the hold window is set to. An
    // earlier default of 60_000 quietly meant "quiet" only because the window
    // was 3s, so widening the window silently changed what these tests asserted.
    msSinceInput: 1000, msSinceOutput: Infinity,
    selectedActiveMs: 60_000, finishSeq: 0, recentFinishMs: [],
    firstRunOfDay: false, freshInstall: false,
    ...over,
  };
}

test("selectedSession finds the selected windowID, or null", () => {
  assert.equal(selectedSession(makeState()).windowID, "w1");
  assert.equal(selectedSession({ ...makeState(), selected: "w9" }), null);
  assert.equal(selectedSession(null), null);
  assert.equal(selectedSession({ sessions: null, selected: "w1" }), null);
});

test("error outranks everything while inside ERROR_TTL_MS", () => {
  const state = makeState({ errorMs: NOW - 1000, lastFinishMs: NOW - 100 });
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: 0 })), "error");
  // boundary: exactly TTL has elapsed -> no longer error
  const stale = makeState({ errorMs: NOW - ERROR_TTL_MS });
  assert.equal(pickSlot(stale, makeFacts()), "idle");
});

test("done holds for DONE_TTL_MS then hands over to awaiting", () => {
  const at = (age) => pickSlot(makeState({ lastFinishMs: NOW - age }), makeFacts());
  assert.equal(at(0), "done");
  assert.equal(at(DONE_TTL_MS - 1), "done");
  assert.equal(at(DONE_TTL_MS), "awaiting");   // finish is still unconsumed
  assert.equal(at(60_000), "awaiting");
});

test("a second finish inside DONE_TTL restarts the window", () => {
  // The room debounces the transition (spec §5.6); the ladder simply reports
  // `done` again from the newer timestamp.
  const first = makeState({ lastFinishMs: NOW - 4000 });
  assert.equal(pickSlot(first, makeFacts()), "done");
  const second = makeState({ lastFinishMs: NOW - 100 });
  assert.equal(pickSlot(second, makeFacts({ nowMs: NOW })), "done");
});

test("awaiting requires an unconsumed finish: any later output clears it", () => {
  const state = makeState({ lastFinishMs: NOW - 60_000 });
  // last output was 90s ago, i.e. BEFORE the finish -> nothing since -> awaiting
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: 90_000 })), "awaiting");
  // last output 10s ago, i.e. AFTER the finish -> the finish was consumed, and
  // a turn is in flight again
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: 10_000 })), "working");
});

test("working holds across a quiet gap inside one turn", () => {
  const state = makeState();
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: 0 })), "working");
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: WORKING_HOLD_MS - 1 })), "working");
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: WORKING_HOLD_MS })), "idle");
});

// The defect this window exists for. Claude Code prints nothing for tens of
// seconds inside a SINGLE turn — a long tool call, a build, a think — and the
// previous 3s window called every one of those gaps "stopped". With an 8s slot
// dwell on top, the figure alternated working/idle for as long as anyone
// watched. Walking a realistic gap pattern must yield `working` throughout.
test("a realistic burst-and-pause turn never drops out of working", () => {
  const state = makeState();
  // Expressed as FRACTIONS of the window, not absolute milliseconds. An
  // earlier version hard-coded gaps that happened to sit under a 120s window;
  // shortening the window put one of them exactly on the boundary and the test
  // failed for a reason that had nothing to do with the behaviour it names.
  const gaps = [0, 400, 2_500, 9_000, WORKING_HOLD_MS * 0.5, WORKING_HOLD_MS * 0.9, 3_000, 0, 800];
  for (const msSinceOutput of gaps) {
    assert.equal(
      pickSlot(state, makeFacts({ msSinceOutput })), "working",
      `a ${msSinceOutput}ms gap inside a turn must not read as stopped`,
    );
  }
});

// The window is a backstop, not a latch: a session that genuinely stops, with
// no finish signal to catch it, must still come to rest.
test("a session that truly goes quiet eventually leaves working", () => {
  const state = makeState();
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: WORKING_HOLD_MS + 1 })), "idle");
  assert.equal(
    pickSlot(state, makeFacts({ msSinceOutput: Infinity, msSinceInput: BORED_MS + 1 })), "bored",
    "a pane that never produced output must not be held in working",
  );
});

test("NEITHER working NOR awaiting may derive from status", () => {
  // status is "active" from launch until the Stop hook, so it means "not yet
  // finished", not "doing something" — and SelectSession overwrites it to
  // "active" on selection, which would make awaiting unreachable for any
  // session you click into (spec §5.6, app.go:1427-1435).
  // Beyond the hold window, so anything that still said `working` here could
  // only be deriving it from status.
  const active = makeState({ status: "active" });
  assert.equal(pickSlot(active, makeFacts({ msSinceOutput: WORKING_HOLD_MS + 1 })), "idle",
    "status=active must not produce working");

  const activeButFinished = makeState({ status: "active", lastFinishMs: NOW - 60_000 });
  assert.equal(pickSlot(activeButFinished, makeFacts({ msSinceOutput: 90_000 })), "awaiting",
    "status=active must not suppress awaiting");

  const finishedButStreaming = makeState({ status: "finished", lastFinishMs: 0 });
  assert.equal(pickSlot(finishedButStreaming, makeFacts({ msSinceOutput: 0 })), "working",
    "status=finished must not suppress working");
});

test("bored needs a long input gap and nothing live", () => {
  const state = makeState();
  assert.equal(pickSlot(state, makeFacts({ msSinceInput: BORED_MS })), "idle");
  assert.equal(pickSlot(state, makeFacts({ msSinceInput: BORED_MS + 1 })), "bored");
  // recent output wins: working outranks bored
  assert.equal(pickSlot(state, makeFacts({ msSinceInput: BORED_MS + 1, msSinceOutput: 0 })), "working");
});

test("a background session finishing or erroring never moves the foreground", () => {
  // done/error are per-session fields, not global signals (spec §6.2).
  const bgFinished = makeState({}, { lastFinishMs: NOW - 100, status: "finished" });
  assert.equal(pickSlot(bgFinished, makeFacts()), "idle");
  const bgErrored = makeState({}, { errorMs: NOW - 100 });
  assert.equal(pickSlot(bgErrored, makeFacts()), "idle");
});

test("no selected session falls back to idle/bored on input alone", () => {
  const none = { ...makeState(), selected: "" };
  assert.equal(pickSlot(none, makeFacts()), "idle");
  assert.equal(pickSlot(none, makeFacts({ msSinceInput: BORED_MS + 1 })), "bored");
});

test("every ladder outcome is a member of SLOTS", () => {
  const cases = [
    [makeState({ errorMs: NOW }), makeFacts()],
    [makeState({ lastFinishMs: NOW }), makeFacts()],
    [makeState({ lastFinishMs: NOW - 60_000 }), makeFacts({ msSinceOutput: 90_000 })],
    [makeState(), makeFacts({ msSinceOutput: 0 })],
    [makeState(), makeFacts({ msSinceInput: BORED_MS + 1 })],
    [makeState(), makeFacts()],
  ];
  const seen = new Set(cases.map(([s, f]) => pickSlot(s, f)));
  assert.deepEqual([...seen].sort(), ["awaiting", "bored", "done", "error", "idle", "working"]);
});

test("infinite msSinceOutput (a session that never produced output) is safe", () => {
  const state = makeState({ lastFinishMs: NOW - 1000 });
  assert.equal(pickSlot(state, makeFacts({ msSinceOutput: Infinity })), "done");
  const older = makeState({ lastFinishMs: NOW - 60_000 });
  assert.equal(pickSlot(older, makeFacts({ msSinceOutput: Infinity })), "awaiting");
});

// --- the ladder and the dwell, composed --------------------------------------

// Both halves were individually correct and the combination was not. DONE_TTL
// used to be 5s against an 8s dwell, so a turn finishing 3s into a dwell
// expired before the gate could adopt it: the gate held `working` to t=8s and
// by then the candidate was already `awaiting`. The celebratory image the user
// paid for was unreachable for every turn shorter than the dwell.
//
// The host composes these two; so must a test. Neither half catches it alone.
test("a short turn still reaches done, at the dwell boundary", () => {
  const dwell = createDwell();
  const state = makeState();
  const t0 = NOW;

  for (let ms = 0; ms <= 3000; ms += 500) {
    const slot = pickSlot(state, makeFacts({ nowMs: t0 + ms, msSinceOutput: 100 }));
    assert.equal(dwell.gate(slot, t0 + ms), "working");
  }

  // Finishes at t=3s. The dwell rightly holds `working` for now — showing the
  // celebration instantly is what would reintroduce the slideshow.
  const finished = makeState({ lastFinishMs: t0 + 3000 });
  const held = dwell.gate(
    pickSlot(finished, makeFacts({ nowMs: t0 + 3100, msSinceOutput: 200 })), t0 + 3100);
  assert.equal(held, "working", "the dwell should still be protecting the transition");

  // At the boundary the candidate must STILL be `done`, so the gate adopts it
  // rather than skipping past to `awaiting`.
  const atBoundary = pickSlot(finished, makeFacts({ nowMs: t0 + SLOT_DWELL_MS, msSinceOutput: 200 }));
  assert.equal(atBoundary, "done", "done expired before the dwell could adopt it");
  assert.equal(dwell.gate(atBoundary, t0 + SLOT_DWELL_MS), "done");
});

// After a turn longer than the dwell — the common case — the gate is already
// free, so the celebration is immediate.
test("a long turn shows done straight away", () => {
  const dwell = createDwell();
  const t0 = NOW;
  dwell.gate("working", t0);

  const finished = makeState({ lastFinishMs: t0 + 30_000 });
  const slot = pickSlot(finished, makeFacts({ nowMs: t0 + 30_100, msSinceOutput: 200 }));
  assert.equal(slot, "done");
  assert.equal(dwell.gate(slot, t0 + 30_100), "done", "the gate was free and should have adopted it");
});

// error must still cut in — it is the one thing worth interrupting for.
test("error still preempts the dwell", () => {
  const dwell = createDwell();
  dwell.gate("working", NOW);
  assert.equal(dwell.gate("error", NOW + 100), "error");
});
