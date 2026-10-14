import { test } from "node:test";
import assert from "node:assert/strict";
import { stageOf, floorsOf, cranesOf, workersOf, propOf, poseOf, archetypeOfRow, compacting } from "./sites.js";

test("stage from context fill", () => {
  assert.deepEqual([0, 24, 25, 49, 50, 79, 80, 100].map(stageOf), [0, 0, 1, 1, 2, 2, 3, 3]);
  assert.equal(stageOf(null), 1);
});

test("floors, cranes, workers", () => {
  assert.equal(floorsOf(0), 0); assert.equal(floorsOf(45), 2);
  const crew = Array.from({ length: 5 }, (_, i) => ({ state: i < 4 ? "tool" : "finished" }));
  assert.deepEqual(cranesOf(crew), { shown: 3, more: 1 });
  const now = 100000;
  const evs = Array.from({ length: 20 }, (_, i) => ({ at: now - i * 1000, kind: "read" }));
  evs.push({ at: now - 1000, kind: "think" }, { at: now - 120000, kind: "edit" }, { at: now - 1000, kind: "read", agent: "a" });
  assert.equal(workersOf(evs, now), 12);
});

test("tool props", () => {
  const r = (tool, target = "") => ({ activity: "tool", tool, target });
  assert.equal(propOf(r("Read")), "truck"); assert.equal(propOf(r("WebFetch")), "truck");
  assert.equal(propOf(r("Write")), "mixer"); assert.equal(propOf(r("Edit")), "sparks");
  assert.equal(propOf(r("Bash", "ls")), "drill"); assert.equal(propOf(r("Bash", "go test ./...")), "inspector");
  assert.equal(propOf({ activity: "thinking" }), null);
});

test("poses follow state and wait time", () => {
  const now = 1_000_000;
  const p = (activity, since, extra = {}) => poseOf({ activity, sinceMs: now - since, unread: false, ...extra }, now);
  assert.equal(p("permission", 60000), "stop"); assert.equal(p("permission", 300000), "lunch"); assert.equal(p("permission", 700000), "nap");
  assert.equal(p("error", 0), "fire"); assert.equal(p("finished", 0, { unread: true }), "ribbon");
  assert.equal(p("finished", 0), "dusk"); assert.equal(p("idle", 0), "dusk"); assert.equal(p("tool", 0), "work");
});

test("archetype from provider, then model", () => {
  assert.equal(archetypeOfRow({ provider: "anthropic" }), "tower");
  assert.equal(archetypeOfRow({ provider: "ollama-cloud" }), "block");
  assert.equal(archetypeOfRow({ provider: "bedrock" }), "hall");
  assert.equal(archetypeOfRow({ provider: "", model: "claude-opus-5-5" }), "tower");
  assert.equal(archetypeOfRow({ provider: "", model: "glm" }), "hall");
});

test("compaction pose lasts 60 s", () => {
  assert.equal(compacting({ compactedMs: 0 }, 1000), null);
  assert.equal(compacting({ compactedMs: 1000 }, 1000), 0);
  assert.ok(compacting({ compactedMs: 1000 }, 31000) > 0.4);
  assert.equal(compacting({ compactedMs: 1000 }, 62000), null);
});

// Minors round 2 (phase 5a review): the ribbon's fireworks looped for as long as
// a finished session stayed unread. They burst only just after the finish.
test("fireworksOf: only in the first 15 s after an unread finish", async () => {
  const { fireworksOf } = await import("./sites.js");
  const r = (sinceMs, extra = {}) => ({ activity: "finished", unread: true, sinceMs, ...extra });
  assert.equal(fireworksOf(r(100_000), 105_000), true);
  assert.equal(fireworksOf(r(100_000), 200_000), false);
  assert.equal(fireworksOf(r(100_000, { unread: false }), 101_000), false);
  assert.equal(fireworksOf(r(0), 5_000), false);
});
