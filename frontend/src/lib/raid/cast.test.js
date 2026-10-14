// frontend/src/lib/raid/cast.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { heroineOf, bossOf, bossLevel, tierOf, armourOf, moveOf, allyOf, partyOf, clip } from "./cast.js";

test("heroine by model family, case-insensitive", () => {
  const ids = ["claude-opus-5-5", "Sonnet 5.5", "HAIKU", "glm-5.3", "qwen3.5:397b", "kimi-k2"].map((m) => heroineOf({ model: m, id: "s" }).id);
  assert.deepEqual(ids, ["opus", "sonnet", "haiku", "glm", "qwen", "kimi"]);
  assert.equal(heroineOf({ model: "opus", id: "s" }).name, "Archmage");
  assert.equal(heroineOf({ model: "opus", id: "s" }).mercenary, false);
});

test("unknown model is a stable Mercenary", () => {
  const a = heroineOf({ model: "gpt-9", id: "abc" });
  const b = heroineOf({ model: "", id: "abc" });
  assert.equal(a.name, "Mercenary"); assert.equal(a.mercenary, true);
  assert.equal(a.id, b.id);
  assert.ok(["opus", "sonnet", "haiku", "glm", "qwen", "kimi"].includes(a.id));
  assert.equal(heroineOf({ key: "ext:x" }).mercenary, true); // no model, no id
});

test("boss by title keyword, first rule wins, whole words only", () => {
  assert.equal(bossOf({ title: "Fix the crash on save" }), "hydra");
  assert.equal(bossOf({ title: "Refactor legacy auth" }), "golem");
  assert.equal(bossOf({ title: "CI release pipeline" }), "kraken");
  assert.equal(bossOf({ title: "SQL schema tidy" }), "wraith");
  assert.equal(bossOf({ title: "Brainstorm the UI" }), "sphinx");
  assert.equal(bossOf({ title: "fix the UI" }), "hydra"); // hydra rule is first
  assert.equal(bossOf({ title: "prefix builder", id: "q" }), bossOf({ title: "zzz", id: "q" })); // "prefix" ≠ "fix", "builder" ≠ "build"
});

test("boss level from code and complexity, clamped", () => {
  assert.equal(bossLevel(null), 1);
  assert.equal(bossLevel({ code: 0, complexity: 0 }), 1);
  assert.equal(bossLevel({ code: 10000, complexity: 400 }), 29);
  assert.equal(bossLevel({ code: 1e9, complexity: 1e6 }), 99);
});

test("tier: slime, beast, raid with 2 or 3 pips; no metrics is a beast", () => {
  assert.deepEqual(tierOf({ code: 1999 }), { tier: "slime", pips: 0 });
  assert.deepEqual(tierOf({ code: 2000 }), { tier: "beast", pips: 0 });
  assert.deepEqual(tierOf({ code: 30000 }), { tier: "raid", pips: 2 });
  assert.deepEqual(tierOf({ code: 100000 }), { tier: "raid", pips: 3 });
  assert.deepEqual(tierOf(null), { tier: "beast", pips: 0 });
});

test("armour pips from test ratio", () => {
  assert.equal(armourOf(null), 0);
  assert.equal(armourOf({ testRatio: 0 }), 0);
  assert.equal(armourOf({ testRatio: 0.25 }), 2); // round(1.5) = 2
  assert.equal(armourOf({ testRatio: 0.9 }), 3);
});

test("moves by tool", () => {
  assert.equal(moveOf("Edit"), "Refactor Strike");
  assert.equal(moveOf("Write"), "Forge");
  assert.equal(moveOf("Grep"), "Seek");
  assert.equal(moveOf("Bash"), "Engineer's Blast");
  assert.equal(moveOf("Task"), "Summon");
  assert.equal(moveOf("WebSearch"), "Far Sight");
  assert.equal(moveOf("TodoWrite"), "Battle Plan");
  assert.equal(moveOf("mcp__x"), "Strike");
  assert.equal(moveOf(""), "Strike");
});

test("allies by agent type and size by model; party caps at 6 and depth 2", () => {
  assert.deepEqual(allyOf({ type: "Explore", model: "haiku" }), { kind: "fox", size: 0.7 });
  assert.deepEqual(allyOf({ type: "Plan", model: "sonnet" }), { kind: "strategist", size: 0.85 });
  assert.deepEqual(allyOf({ type: "code-reviewer", model: "opus" }), { kind: "cleric", size: 1 });
  assert.deepEqual(allyOf({ type: "general-purpose" }), { kind: "knight", size: 1 });
  const crew = Array.from({ length: 9 }, (_, i) => ({ id: "a" + i, type: "Explore", depth: i === 0 ? 3 : 1 }));
  const p = partyOf(crew);
  assert.equal(p.allies.length, 6);
  assert.equal(p.more, 3);
  assert.ok(p.allies.every((a) => a.kind === "fox" && a.id !== "a0"));
});

test("clip counts characters, not bytes, and marks the cut", () => {
  assert.equal(clip("abc", 80), "abc");
  assert.equal(clip("x".repeat(500), 80).length, 80);
  assert.ok(clip("x".repeat(500), 80).endsWith("…"));
  assert.equal(clip(undefined, 5), "");
});

test("heroineOf takes name and tint from the pack", () => {
  const pack = { files: {}, heroines: { opus: { name: "Layla", tint: "#112233" }, kimi: { tint: "#445566" } } };
  const h = heroineOf({ model: "claude-opus-5-5" }, pack);
  assert.equal(h.name, "Layla");
  assert.equal(h.tint, "#112233");
  const m = heroineOf({ model: "gpt-x", id: "zz" }, pack);
  assert.equal(m.name, "Mercenary");
  assert.equal(heroineOf({ model: "kimi-k2" }, pack).name, "Mecha Pilot");
  assert.equal(heroineOf({ model: "kimi-k2" }, pack).tint, "#445566");
});
