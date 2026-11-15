import { test } from "node:test";
import assert from "node:assert/strict";
import { rangeFor, previous, dayKey } from "./range.js";

const now = new Date(2026, 10, 12, 15, 0); // 12 Nov 2026 local

test("today / 7d / 30d", () => {
  assert.deepEqual(rangeFor("today", now), { from: "2026-11-12", to: "2026-11-12", days: 1 });
  assert.deepEqual(rangeFor("7d", now), { from: "2026-11-06", to: "2026-11-12", days: 7 });
  assert.equal(rangeFor("30d", now).from, "2026-10-14");
});
test("all starts at the first ledger day, else open", () => {
  assert.deepEqual(rangeFor("all", now, "2026-09-01"), { from: "2026-09-01", to: "2026-11-12", days: 73 });
  assert.equal(rangeFor("all", now).from, "");
});
test("custom range", () => assert.deepEqual(rangeFor({ from: "2026-11-01", to: "2026-11-03" }, now), { from: "2026-11-01", to: "2026-11-03", days: 3 }));
test("previous is the equal span just before", () =>
  assert.deepEqual(previous({ from: "2026-11-06", to: "2026-11-12", days: 7 }), { from: "2026-10-30", to: "2026-11-05" }));
test("day keys stay contiguous across a DST change", () => {
  const keys = [];
  for (let d = new Date(2026, 2, 27, 12); d < new Date(2026, 3, 2); d.setDate(d.getDate() + 1)) keys.push(dayKey(d));
  assert.deepEqual(keys, ["2026-03-27", "2026-03-28", "2026-03-29", "2026-03-30", "2026-03-31", "2026-04-01"]);
  assert.deepEqual(rangeFor("7d", new Date(2026, 2, 31, 0, 30)), { from: "2026-03-25", to: "2026-03-31", days: 7 });
});
