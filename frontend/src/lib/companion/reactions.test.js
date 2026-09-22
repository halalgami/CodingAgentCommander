import { test } from "node:test";
import assert from "node:assert/strict";
import { Reactions } from "./reactions.js";

const base = { running: 0, finished: 0, awaiting: false, error: false, quotaHigh: false, greeted: true, idleMs: 0, nowMs: 1000, click: null, rc: false };

test("error transition fires slump once, not while error persists", () => {
  const r = new Reactions();
  r.decide({ ...base, nowMs: 1000 });                       // prime prev
  const a = r.decide({ ...base, error: true, nowMs: 2000 }); // rising edge
  assert.equal(a.gesture, "slump");
  assert.equal(a.expression, "surprised");
  const b = r.decide({ ...base, error: true, nowMs: 3000 }); // still error
  assert.equal(b.gesture, null);
});

test("finished count rising fires wave + happy", () => {
  const r = new Reactions();
  r.decide({ ...base, finished: 0, nowMs: 1000 });
  const a = r.decide({ ...base, finished: 1, nowMs: 2000 });
  assert.equal(a.gesture, "wave");
  assert.equal(a.expression, "happy");
});

test("click regions map to side-aware gestures", () => {
  const r = new Reactions();
  assert.deepEqual(r.decide({ ...base, click: "head", nowMs: 1000 }), { gesture: "headpat", expression: "happy" });
  assert.equal(r.decide({ ...base, click: "handL", nowMs: 2000 }).gesture, "waveL");
  assert.equal(r.decide({ ...base, click: "handR", nowMs: 3000 }).gesture, "waveR");
  assert.equal(r.decide({ ...base, click: "armL", nowMs: 4000 }).gesture, "nudgeL");
  assert.equal(r.decide({ ...base, click: "legR", nowMs: 5000 }).gesture, "kickR");
  assert.ok(["tickle", "squirm"].includes(r.decide({ ...base, click: "belly", nowMs: 6000 }).gesture));
  const hips = r.decide({ ...base, click: "hips", nowMs: 7000 });
  assert.equal(hips.gesture, "wiggle");
  assert.equal(hips.expression, "surprised");
});

test("cooldown suppresses a second gesture within 600ms", () => {
  const r = new Reactions();
  r.decide({ ...base, click: "handL", nowMs: 1000 });          // fires waveL
  const b = r.decide({ ...base, click: "chest", nowMs: 1300 }); // within cooldown
  assert.equal(b.gesture, null);
});

test("quotaHigh sets sad baseline when nothing else fires", () => {
  const r = new Reactions();
  const a = r.decide({ ...base, quotaHigh: true, nowMs: 9000 });
  assert.equal(a.expression, "sad");
});

test("running sessions set relaxed baseline", () => {
  const r = new Reactions();
  const a = r.decide({ ...base, running: 2, nowMs: 9000 });
  assert.equal(a.expression, "relaxed");
});

test("ungreeted fires greet on first decide", () => {
  const r = new Reactions();
  const a = r.decide({ ...base, greeted: false, nowMs: 500 });
  assert.equal(a.gesture, "greet");
});

test("pooled hotspot picks a member gesture deterministically by rng", () => {
  const r = new Reactions(() => 0);      // rng -> always first
  const a = r.decide({ ...base, click: "belly", nowMs: 1000 });
  const r2 = new Reactions(() => 0.99);  // rng -> last
  const b = r2.decide({ ...base, click: "belly", nowMs: 1000 });
  assert.ok(["tickle", "squirm"].includes(a.gesture));
  assert.ok(["tickle", "squirm"].includes(b.gesture));
  // singleton pool (head) always returns headpat regardless of rng
  assert.equal(new Reactions(() => 0.7).decide({ ...base, click: "head", nowMs: 2000 }).gesture, "headpat");
});
