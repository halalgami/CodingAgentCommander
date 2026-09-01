import { test } from "node:test";
import assert from "node:assert/strict";
import { localDayStamp, computeFirstRunOfDay } from "./dayRollover.js";

const DAY_MS = 24 * 60 * 60 * 1000;

function fakeStorage() {
  const m = new Map();
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => { m.set(k, String(v)); },
  };
}

test("localDayStamp changes only when the local calendar day changes", () => {
  const noon = new Date(2026, 0, 15, 12, 0, 0).getTime();
  const lateSameDay = new Date(2026, 0, 15, 23, 59, 59).getTime();
  const nextDay = new Date(2026, 0, 16, 0, 0, 1).getTime();
  assert.equal(localDayStamp(noon), localDayStamp(lateSameDay));
  assert.notEqual(localDayStamp(noon), localDayStamp(nextDay));
});

test("the first call of a fresh storage is true exactly once, then false the same day", () => {
  const storage = fakeStorage();
  const now = Date.now();
  assert.equal(computeFirstRunOfDay(now, storage), true);
  assert.equal(computeFirstRunOfDay(now, storage), false, "a second call the same instant must not re-fire");
  assert.equal(computeFirstRunOfDay(now + 1000, storage), false, "a minute later, same day, still false");
  assert.equal(computeFirstRunOfDay(now + 60_000, storage), false);
});

test("crossing a local-day boundary flips it true again exactly once", () => {
  const storage = fakeStorage();
  const day1 = new Date(2026, 5, 1, 9, 0, 0).getTime();
  const day2 = new Date(2026, 5, 2, 0, 30, 0).getTime();
  const day3 = new Date(2026, 5, 3, 8, 0, 0).getTime();
  assert.equal(computeFirstRunOfDay(day1, storage), true);
  assert.equal(computeFirstRunOfDay(day1 + 1000, storage), false);
  assert.equal(computeFirstRunOfDay(day2, storage), true, "the day rolled over");
  assert.equal(computeFirstRunOfDay(day2 + 1000, storage), false);
  assert.equal(computeFirstRunOfDay(day3, storage), true, "another rollover, another single true");
  assert.equal(computeFirstRunOfDay(day3 + 1000, storage), false);
});

test("a persisted stamp from an earlier process run is honoured on the very first call", () => {
  const storage = fakeStorage();
  const yesterday = Date.now() - DAY_MS;
  storage.setItem("cc.last-run-day", localDayStamp(yesterday));
  assert.equal(computeFirstRunOfDay(Date.now(), storage), true, "yesterday's stamp must not suppress today");
});

test("a storage that throws on read/write degrades to true every call, not a crash", () => {
  const angry = {
    getItem() { throw new Error("blocked"); },
    setItem() { throw new Error("blocked"); },
  };
  assert.equal(computeFirstRunOfDay(Date.now(), angry), true);
  assert.equal(computeFirstRunOfDay(Date.now(), angry), true);
});
