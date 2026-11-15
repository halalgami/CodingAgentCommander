import { test } from "node:test";
import assert from "node:assert/strict";
import { pillarOf, shareOf } from "./worldmath.js";

test("pillarOf scales height and alpha with tokens against the busiest project", () => {
  assert.deepEqual(pillarOf({ tokens: 1_200_000, cost: 34, unpriced: 0 }, 1_200_000), { h: 120, alpha: 1, label: "1.2M tok · ≈ $34.00" });
  const z = pillarOf({ tokens: 0, cost: 0, unpriced: 0 }, 100);
  assert.equal(z.h, 12);
  assert.equal(z.alpha, 0.35);
});

test("pillarOf: missing project or no max → null; unpriced only → tokens, never $", () => {
  assert.equal(pillarOf(undefined, 100), null);
  assert.equal(pillarOf({ tokens: 5, cost: 0, unpriced: 5 }, 0), null);
  assert.equal(pillarOf({ tokens: 1_200_000, cost: 0, unpriced: 1_200_000 }, 2_400_000).label, "1.2M tok");
});

test("shareOf is 0..1 and 0 when max ≤ 0", () => {
  assert.equal(shareOf(5, 0), 0);
  assert.equal(shareOf(5, 10), 0.5);
  assert.equal(shareOf(20, 10), 1);
});
