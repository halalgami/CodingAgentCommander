import { test } from "node:test";
import assert from "node:assert/strict";
import {
  SLOT_DWELL_MS, localParts, buildFacts, pruneFinishes, withLatchedError, createDwell,
} from "./facts.js";
import { ERROR_TTL_MS } from "./pack.js";
import { STREAK_WINDOW_MS } from "./conditions.js";

const NOW = 1_700_000_000_000;
const parts = () => ({ hour: 14, weekday: 3 });

function makeState(sel = {}, over = {}) {
  return {
    sessions: [
      { windowID: "w1", name: "alpha", status: "active", statusSinceMs: NOW - 60_000,
        lastFinishMs: 0, errorMs: 0, ...sel },
      { windowID: "w2", name: "beta", status: "active", statusSinceMs: NOW - 10_000,
        lastFinishMs: 0, errorMs: 0 },
    ],
    selected: "w1",
    running: 2, finished: 0, finishSeq: 7, lastFinished: "",
    ...over,
  };
}

test("buildFacts produces EVERY field the decision core reads, none undefined", () => {
  // A missing field is not a crash, it is silence: every comparison against
  // undefined is false and the whole system collapses to one constant.
  const f = buildFacts({
    state: makeState(), nowMs: NOW, msSinceInput: 1000, msSinceOutput: 500, parts,
  });
  assert.deepEqual(Object.keys(f).sort(), [
    "finishSeq", "firstRunOfDay", "freshInstall", "hour", "msSinceInput",
    "msSinceOutput", "nowMs", "recentFinishMs", "selectedActiveMs", "weekday",
  ]);
  for (const [k, v] of Object.entries(f)) assert.notEqual(v, undefined, `${k} is undefined`);
});

test("buildFacts derives selectedActiveMs from the SELECTED session's statusSinceMs", () => {
  // statusSinceMs is the last status transition, not the session start, so
  // `marathon` cannot fire for a session launched 61 minutes ago that finished
  // and restarted two minutes ago (spec §6.2).
  const f = buildFacts({ state: makeState(), nowMs: NOW, msSinceInput: 0, msSinceOutput: 0, parts });
  assert.equal(f.selectedActiveMs, 60_000);
  const other = buildFacts({
    state: { ...makeState(), selected: "w2" }, nowMs: NOW, msSinceInput: 0, msSinceOutput: 0, parts,
  });
  assert.equal(other.selectedActiveMs, 10_000);
});

test("buildFacts with no selected session yields zero active time, not NaN", () => {
  const f = buildFacts({
    state: { ...makeState(), selected: "" }, nowMs: NOW, msSinceInput: 0, msSinceOutput: 0, parts,
  });
  assert.equal(f.selectedActiveMs, 0);
  assert.equal(f.finishSeq, 7);
});

test("buildFacts passes Infinity through: a never-run pane is not a live pane", () => {
  const f = buildFacts({
    state: makeState(), nowMs: NOW, msSinceInput: Infinity, msSinceOutput: Infinity, parts,
  });
  assert.equal(f.msSinceOutput, Infinity);
  assert.equal(f.msSinceInput, Infinity);
});

test("hour and weekday come from the injected parts function, never a real clock", () => {
  const f = buildFacts({
    state: makeState(), nowMs: NOW, msSinceInput: 0, msSinceOutput: 0,
    parts: () => ({ hour: 3, weekday: 6 }),
  });
  assert.equal(f.hour, 3);
  assert.equal(f.weekday, 6);
});

test("localParts reads the LOCAL hour and weekday of a timestamp", () => {
  const d = new Date(NOW);
  assert.deepEqual(localParts(NOW), { hour: d.getHours(), weekday: d.getDay() });
});

test("pruneFinishes keeps only the streak window and drops the rest", () => {
  // `streak` is 3 finishes within 10 minutes; conditions.js counts the array
  // length, so the host owns the windowing (conditions.js STREAK_WINDOW_MS).
  const list = [NOW - STREAK_WINDOW_MS - 1, NOW - STREAK_WINDOW_MS, NOW - 1000, NOW];
  assert.deepEqual(pruneFinishes(list, NOW), [NOW - 1000, NOW]);
  assert.deepEqual(pruneFinishes(null, NOW), []);
  assert.deepEqual(pruneFinishes(["x", NaN, NOW], NOW), [NOW]);
});

test("buildFacts prunes finishTimes into recentFinishMs", () => {
  const f = buildFacts({
    state: makeState(), nowMs: NOW, msSinceInput: 0, msSinceOutput: 0, parts,
    finishTimes: [NOW - STREAK_WINDOW_MS - 5, NOW - 5],
  });
  assert.deepEqual(f.recentFinishMs, [NOW - 5]);
});

test("withLatchedError attributes a fresh app:error to the selected session", () => {
  // reportError already emits app:error to the deck (app.go:352). Latching it
  // frontend-side needs no new Go state and no binding regeneration (spec §6.4).
  const out = withLatchedError(makeState(), NOW - 1000, NOW);
  assert.equal(out.sessions.find((s) => s.windowID === "w1").errorMs, NOW - 1000);
  assert.equal(out.sessions.find((s) => s.windowID === "w2").errorMs, 0,
    "the latch must not smear across background sessions");
});

test("withLatchedError ignores an error older than ERROR_TTL_MS", () => {
  const out = withLatchedError(makeState(), NOW - ERROR_TTL_MS, NOW);
  assert.equal(out.sessions.find((s) => s.windowID === "w1").errorMs, 0);
});

test("withLatchedError never mutates the input and keeps the newer timestamp", () => {
  const src = makeState({ errorMs: NOW - 100 });
  const out = withLatchedError(src, NOW - 5000, NOW);
  assert.equal(src.sessions[0].errorMs, NOW - 100, "input state was mutated");
  assert.equal(out.sessions[0].errorMs, NOW - 100, "the older latch overwrote a newer per-session error");
  assert.notEqual(out, src);
});

test("withLatchedError is a no-op with no latch and no selection", () => {
  const src = makeState();
  assert.equal(withLatchedError(src, 0, NOW), src);
  assert.equal(withLatchedError({ ...src, selected: "" }, NOW, NOW).sessions[0].errorMs, 0);
});

test("the dwell adopts the first slot immediately", () => {
  const d = createDwell();
  assert.equal(d.gate("idle", NOW), "idle");
  assert.equal(d.held(), "idle");
});

test("the dwell holds a slot for SLOT_DWELL_MS before the next one can show", () => {
  assert.equal(SLOT_DWELL_MS, 8000);
  const d = createDwell();
  d.gate("working", NOW);
  assert.equal(d.gate("done", NOW + SLOT_DWELL_MS - 1), "working");
  assert.equal(d.gate("done", NOW + SLOT_DWELL_MS), "done");
});

test("the dwell kills the working -> done -> awaiting -> working slideshow", () => {
  // Without it, exactly the sessions people watch produce a slideshow (§4.3).
  const d = createDwell();
  d.gate("working", NOW);
  const shown = new Set();
  const script = ["done", "awaiting", "working", "done", "awaiting", "working"];
  script.forEach((slot, i) => shown.add(d.gate(slot, NOW + 500 * (i + 1))));
  assert.deepEqual([...shown], ["working"], "the region changed slot inside the dwell");
});

test("the dwell adopts whatever is current when it expires, not what first differed", () => {
  // That is the hysteresis: a slot that flickered at t+1s and went away must not
  // be the thing that lands at t+8s.
  const d = createDwell();
  d.gate("working", NOW);
  d.gate("done", NOW + 1000);      // flickers
  d.gate("working", NOW + 2000);   // and goes away
  assert.equal(d.gate("bored", NOW + SLOT_DWELL_MS), "bored");
});

test("re-reporting the held slot does not restart its dwell", () => {
  const d = createDwell();
  d.gate("idle", NOW);
  for (let t = 0; t < SLOT_DWELL_MS; t += 500) d.gate("idle", NOW + t);
  assert.equal(d.gate("working", NOW + SLOT_DWELL_MS), "working");
});

test("error preempts the dwell, because an error is the one slot with information", () => {
  const d = createDwell();
  d.gate("working", NOW);
  assert.equal(d.gate("error", NOW + 100), "error");
  // and adopting it restarts the dwell, so error itself cannot flicker
  assert.equal(d.gate("working", NOW + 200), "error");
});
