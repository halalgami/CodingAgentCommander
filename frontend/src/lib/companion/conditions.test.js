import { test } from "node:test";
import assert from "node:assert/strict";
import { evaluate, isEdge, MARATHON_MS, LONG_IDLE_MS } from "./conditions.js";

// A fully-populated facts object. Every test derives from this by spread, so a
// field added to the shape later cannot silently read as undefined here.
const facts = {
  nowMs: 1_700_000_000_000,
  hour: 14,
  weekday: 3,
  msSinceInput: 1000,
  msSinceOutput: 1000,
  selectedActiveMs: 0,
  finishSeq: 0,
  recentFinishMs: [],
  firstRunOfDay: false,
  freshInstall: false,
};

test("isEdge splits the two condition classes", () => {
  for (const n of ["lateNight", "weekend", "marathon", "longIdle", "freshInstall"]) {
    assert.equal(isEdge(n), false, `${n} must be ambient`);
  }
  for (const n of ["firstRunOfDay", "century", "streak"]) {
    assert.equal(isEdge(n), true, `${n} must be an event`);
  }
  assert.equal(isEdge("nonsense"), false);
});

test("lateNight is [2, 5) local hours", () => {
  const at = (hour) => evaluate("lateNight", { ...facts, hour }, null);
  assert.equal(at(1), false);
  assert.equal(at(2), true);   // inclusive lower bound
  assert.equal(at(4), true);
  assert.equal(at(5), false);  // exclusive upper bound
  assert.equal(at(23), false);
});

test("weekend is Saturday or Sunday", () => {
  const at = (weekday) => evaluate("weekend", { ...facts, weekday }, null);
  assert.equal(at(0), true);   // Sunday
  assert.equal(at(1), false);
  assert.equal(at(5), false);
  assert.equal(at(6), true);   // Saturday
});

test("marathon is a selected session active longer than an hour", () => {
  assert.equal(MARATHON_MS, 60 * 60 * 1000);
  assert.equal(evaluate("marathon", { ...facts, selectedActiveMs: MARATHON_MS - 1 }, null), false);
  assert.equal(evaluate("marathon", { ...facts, selectedActiveMs: MARATHON_MS + 1 }, null), true);
});

test("longIdle is half an hour without input", () => {
  assert.equal(LONG_IDLE_MS, 30 * 60 * 1000);
  assert.equal(evaluate("longIdle", { ...facts, msSinceInput: LONG_IDLE_MS - 1 }, null), false);
  assert.equal(evaluate("longIdle", { ...facts, msSinceInput: LONG_IDLE_MS + 1 }, null), true);
});

test("freshInstall is a plain flag from the host", () => {
  assert.equal(evaluate("freshInstall", { ...facts, freshInstall: true }, null), true);
  assert.equal(evaluate("freshInstall", facts, null), false);
});

test("ambient conditions are levels: they stay true tick after tick", () => {
  // This is the property that stops lateNight art flashing for 12s every
  // minute instead of being the night's art (spec §5.5).
  const night = { ...facts, hour: 3 };
  let prev = null;
  for (let i = 0; i < 500; i++) {
    assert.equal(evaluate("lateNight", night, prev), true, `tick ${i} went false`);
    prev = night;
  }
});

test("unknown condition names never fire", () => {
  assert.equal(evaluate("someTypo", facts, null), false);
  assert.equal(evaluate(undefined, facts, null), false);
});

test("firstRunOfDay fires on the first tick and never again in that run", () => {
  const on = { ...facts, firstRunOfDay: true };
  assert.equal(evaluate("firstRunOfDay", on, null), true);   // launch tick
  assert.equal(evaluate("firstRunOfDay", on, on), false);    // still the same run
  let prev = on;
  for (let i = 0; i < 100; i++) {
    assert.equal(evaluate("firstRunOfDay", on, prev), false, `re-fired at tick ${i}`);
    prev = on;
  }
});

test("century fires on the crossing, not for every tick the counter sits on 100", () => {
  // The regression this pins: `finishSeq % 100 === 0` stays true until the next
  // finish, so a modulo implementation re-fires ~120 times a minute (spec §5.5).
  const at = (finishSeq) => ({ ...facts, finishSeq });
  assert.equal(evaluate("century", at(100), at(99)), true);   // the crossing
  let fires = 0;
  let prev = at(100);
  for (let i = 0; i < 300; i++) {
    if (evaluate("century", at(100), prev)) fires += 1;
    prev = at(100);
  }
  assert.equal(fires, 0, "century re-fired while the counter sat on 100");
});

test("century needs a real crossing, not just a multiple", () => {
  const at = (finishSeq) => ({ ...facts, finishSeq });
  assert.equal(evaluate("century", at(101), at(100)), false);
  assert.equal(evaluate("century", at(199), at(198)), false);
  assert.equal(evaluate("century", at(200), at(199)), true);
  // a skipped tick still fires exactly once
  assert.equal(evaluate("century", at(305), at(298)), true);
  // no baseline -> no claim
  assert.equal(evaluate("century", at(100), null), false);
  // zero is not a century
  assert.equal(evaluate("century", at(0), at(0)), false);
});

test("streak fires once per rising edge and re-arms when finishes age out", () => {
  const with3 = { ...facts, recentFinishMs: [1, 2, 3] };
  const with2 = { ...facts, recentFinishMs: [2, 3] };
  assert.equal(evaluate("streak", with3, with2), true);    // third finish arrives
  assert.equal(evaluate("streak", with3, with3), false);   // still three, no new edge
  assert.equal(evaluate("streak", with2, with3), false);   // one aged out
  assert.equal(evaluate("streak", with3, with2), true);    // re-armed, fires again
});

test("event conditions ignore a prev of the wrong shape rather than throwing", () => {
  assert.equal(evaluate("streak", { ...facts, recentFinishMs: [1, 2, 3] }, {}), true);
  assert.equal(evaluate("century", { ...facts, finishSeq: 100 }, {}), true);
});
