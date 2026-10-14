import { test } from "node:test";
import assert from "node:assert/strict";
import { motionAllowed } from "./motionRule.js";

const base = { pref: true, reduced: false, lowPower: false, visible: true, focused: true, blurredAt: 0, now: 100000 };

test("auto motion is on by default", () => assert.equal(motionAllowed(base), true));

test("each still condition turns motion off", () => {
  assert.equal(motionAllowed({ ...base, pref: false }), false);
  assert.equal(motionAllowed({ ...base, reduced: true }), false);
  assert.equal(motionAllowed({ ...base, lowPower: true }), false);
  assert.equal(motionAllowed({ ...base, visible: false }), false);
});

test("unfocused keeps motion for 30 s, then goes still", () => {
  assert.equal(motionAllowed({ ...base, focused: false, blurredAt: 100000 - 29000 }), true);
  assert.equal(motionAllowed({ ...base, focused: false, blurredAt: 100000 - 31000 }), false);
});
