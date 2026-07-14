import { test } from "node:test";
import assert from "node:assert/strict";
import { fuzzyScore, fuzzyFilter } from "./fuzzy.js";

test("subsequence matches, non-subsequence doesn't", () => {
  assert.ok(fuzzyScore("cmd", "commander") > 0);
  assert.equal(fuzzyScore("xyz", "commander"), 0);
});

test("consecutive + word-start matches score higher", () => {
  assert.ok(fuzzyScore("com", "commander") > fuzzyScore("cmr", "commander"));
  assert.ok(fuzzyScore("api", "api-server") > fuzzyScore("api", "grapqi"));
});

test("empty query matches everything with base score", () => {
  assert.ok(fuzzyScore("", "anything") > 0);
});

test("fuzzyFilter sorts and drops", () => {
  const items = [{ n: "frontend" }, { n: "api-server" }, { n: "zzz" }];
  const out = fuzzyFilter("ap", items, (i) => i.n);
  assert.equal(out.length, 1);
  assert.equal(out[0].n, "api-server");
});

test("match is case-insensitive", () => {
  assert.ok(fuzzyScore("CMD", "Commander") > 0);
});
