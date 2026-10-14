import { test } from "node:test";
import assert from "node:assert/strict";
import { dayPhase, weatherOf } from "./world.js";

test("day phase from local hour", () => {
  const at = (h) => new Date(2026, 9, 12, h, 0, 0);
  assert.equal(dayPhase(at(12)), "day"); assert.equal(dayPhase(at(19)), "dusk");
  assert.equal(dayPhase(at(6)), "dusk"); assert.equal(dayPhase(at(23)), "night"); assert.equal(dayPhase(at(3)), "night");
});

test("weather: recent API error rains, high quota fogs, else sun", () => {
  const now = 10_000_000;
  assert.equal(weatherOf({ rows: [{ activity: "error", sinceMs: now - 60000 }], quotaPct: null, now }), "rain");
  assert.equal(weatherOf({ rows: [{ activity: "error", sinceMs: now - 700000 }], quotaPct: null, now }), "sun");
  assert.equal(weatherOf({ rows: [], quotaPct: 85, now }), "fog");
  assert.equal(weatherOf({ rows: [], quotaPct: null, now }), "sun");
});
