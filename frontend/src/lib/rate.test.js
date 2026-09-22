import { test } from "node:test";
import assert from "node:assert/strict";
import { trailing } from "./rate.js";

// Real timers, short waits: node:test has no fake-timer facility and the whole
// point of the helper is a wall-clock gap, so the delays here are deliberate.
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

test("collapses a burst into one call carrying the last value", async () => {
  const calls = [];
  const t = trailing((v) => calls.push(v), 20);
  // What a slider drag looks like: many events, no quiet gap.
  for (let i = 160; i <= 520; i++) t(i);
  assert.deepEqual(calls, [], "fired during the burst");
  await sleep(60);
  assert.deepEqual(calls, [520], "should fire once, with the final value");
});

test("runs again after a quiet gap", async () => {
  const calls = [];
  const t = trailing((v) => calls.push(v), 20);
  t(1);
  await sleep(60);
  t(2);
  await sleep(60);
  assert.deepEqual(calls, [1, 2]);
});

test("flush applies the pending value immediately and only once", async () => {
  const calls = [];
  const t = trailing((v) => calls.push(v), 50);
  t(7);
  t.flush();
  assert.deepEqual(calls, [7], "flush should apply at once");
  await sleep(80);
  assert.deepEqual(calls, [7], "the timer must not fire a second time");
});

test("flush with nothing pending does nothing", async () => {
  const calls = [];
  const t = trailing((v) => calls.push(v), 20);
  t.flush();
  await sleep(50);
  assert.deepEqual(calls, []);
});

test("cancel drops the pending value", async () => {
  const calls = [];
  const t = trailing((v) => calls.push(v), 20);
  t(3);
  t.cancel();
  await sleep(50);
  assert.deepEqual(calls, []);
});

test("passes every argument through, not just the first", async () => {
  const calls = [];
  const t = trailing((...a) => calls.push(a), 20);
  t(1, 2, 3);
  t(4, 5, 6); // jiggle takes three
  await sleep(60);
  assert.deepEqual(calls, [[4, 5, 6]]);
});
