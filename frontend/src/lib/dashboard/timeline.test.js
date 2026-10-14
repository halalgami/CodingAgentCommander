import { test } from "node:test";
import assert from "node:assert/strict";
import { appendEvents } from "./timeline.js";

const NOW = 10_000_000;

// Review fix: the frontend cap must match Go's (300 lead + 100 per agent), or
// a burst of agents evicts the lead's glyphs until the next snapshot.
test("caps per lane like the backend, not per session", () => {
  const evs = [];
  for (let i = 0; i < 300; i++) evs.push({ session: "s1", at: NOW - 1000 + i, kind: "read" });
  for (let a = 0; a < 30; a++) for (let i = 0; i < 100; i++) evs.push({ session: "s1", at: NOW - 500 + i, kind: "read", agent: "a" + a });
  const tl = appendEvents({}, evs, NOW);
  assert.equal(tl.s1.filter((e) => !e.agent).length, 300);
  assert.equal(tl.s1.filter((e) => e.agent === "a7").length, 100);
  const more = appendEvents(tl, [{ session: "s1", at: NOW, kind: "edit" }], NOW);
  assert.equal(more.s1.filter((e) => !e.agent).length, 300);
  assert.equal(more.s1.filter((e) => e.agent).length, 3000);
});

test("drops marks older than the window and keeps time order", () => {
  const tl = appendEvents({ s1: [{ session: "s1", at: NOW - 11 * 60000, kind: "read" }] }, [{ session: "s1", at: NOW, kind: "edit" }], NOW);
  assert.deepEqual(tl.s1.map((e) => e.kind), ["edit"]);
});
