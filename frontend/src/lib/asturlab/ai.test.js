import { test } from "node:test";
import assert from "node:assert/strict";
import { weekLabel, answerBars } from "./ai.js";

test("weekLabel same and split months", () => {
  assert.equal(weekLabel("2026-11-09"), "Nov 9–15");
  assert.equal(weekLabel("2026-11-30"), "Nov 30 – Dec 6");
  assert.equal(weekLabel("bad"), "");
});

test("answerBars scales to the largest row, keeps 8, labels prefer label", () => {
  const rows = Array.from({ length: 10 }, (_, i) => ({ key: "k" + i, value: 10 - i }));
  rows[0].label = "Top";
  const b = answerBars({ rows }, 100);
  assert.equal(b.length, 8);
  assert.equal(b[0].w, 100);
  assert.equal(b[0].label, "Top");
  assert.equal(b[1].label, "k1");
  assert.ok(b[7].w < b[1].w);
  assert.deepEqual(answerBars(null), []);
});
