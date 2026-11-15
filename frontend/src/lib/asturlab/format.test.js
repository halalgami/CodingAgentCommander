import { test } from "node:test";
import assert from "node:assert/strict";
import { fmtTokens, fmtUSD, fmtDelta, fmtHours, fmtForecast } from "./format.js";

test("tokens", () => {
  assert.equal(fmtTokens(950), "950");
  assert.equal(fmtTokens(1234), "1.2k");
  assert.equal(fmtTokens(1234567), "1.2M");
  assert.equal(fmtTokens(3.4e9), "3.4B");
});
test("usd is always an estimate", () => {
  assert.equal(fmtUSD(1234.4), "≈ $1,234");
  assert.equal(fmtUSD(3.456), "≈ $3.46");
});
test("delta", () => {
  assert.equal(fmtDelta(110, 100), "+10%");
  assert.equal(fmtDelta(95, 100), "−5%");
  assert.equal(fmtDelta(5, 0), "—");
  assert.equal(fmtDelta(NaN, 3), "—");
  assert.equal(fmtDelta(0, 0), "—");
});
test("hours", () => assert.equal(fmtHours(5_400_000), "1.5 h"));

test("fmtForecast: weekday and hour rounded down, or why there is no forecast", () => {
  const now = new Date(2026, 10, 15, 9, 0); // Sun
  assert.equal(fmtForecast(new Date(2026, 10, 19, 14, 40).toISOString(), now), "At this pace: limit Thu ~14:00");
  assert.equal(fmtForecast("", now, "resets first"), "Won't reach the limit before it resets");
  assert.equal(fmtForecast("", now, "not enough samples"), "Not enough samples yet");
});

test("fmtForecast: a reached limit and a flat window say so (review #2, #3)", () => {
  const now = new Date(2026, 10, 15, 9, 0);
  assert.equal(fmtForecast(new Date(2026, 10, 15, 8, 50).toISOString(), now), "Limit reached");
  assert.equal(fmtForecast("", now, "not rising"), "Not rising — no limit at this pace");
});
