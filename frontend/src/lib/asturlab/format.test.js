import { test } from "node:test";
import assert from "node:assert/strict";
import { fmtTokens, fmtUSD, fmtDelta, fmtHours } from "./format.js";

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
