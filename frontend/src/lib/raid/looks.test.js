// frontend/src/lib/raid/looks.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { lookOf, ascensionOf, gearOf, radarOf, badgesOf, pouchOf, quotaOf, lootOf, motionBudget, wingsOf } from "./looks.js";
import { heroineOf } from "./cast.js";

test("lookOf is stable, in 0..2, and spreads across sessions", () => {
  assert.equal(lookOf({ id: "abc" }), lookOf({ id: "abc" }));
  const seen = new Set(Array.from({ length: 60 }, (_, i) => lookOf({ id: "s" + i })));
  assert.deepEqual([...seen].sort(), [0, 1, 2]);
  assert.equal(lookOf({ id: "", key: "@3" }), lookOf({ key: "@3" }));
});

test("lookOf is independent of the mercenary class hash", () => {
  const pairs = new Set(Array.from({ length: 200 }, (_, i) => {
    const row = { id: "m" + i, model: "mystery" };
    return heroineOf(row).id + ":" + lookOf(row);
  }));
  assert.ok(pairs.size > 12, `only ${pairs.size} class/look pairs`);
});

test("ascension follows hero tier, clamped; no hero → 0", () => {
  assert.equal(ascensionOf(null), 0);
  assert.equal(ascensionOf({ tier: 2 }), 2);
  assert.equal(ascensionOf({ tier: 9 }), 3);
  assert.equal(ascensionOf({ tier: -1 }), 0);
});

test("gear by level: familiar ≥ 3, banner ≥ 8, weapon ≥ 15", () => {
  assert.deepEqual(gearOf(null), []);
  assert.deepEqual(gearOf({ level: 2 }), []);
  assert.deepEqual(gearOf({ level: 3 }), ["familiar"]);
  assert.deepEqual(gearOf({ level: 8 }), ["familiar", "banner"]);
  assert.deepEqual(gearOf({ level: 15 }), ["familiar", "banner", "weapon"]);
});

test("radar scales so the largest axis is 1; empty mix → null", () => {
  assert.equal(radarOf(null), null);
  assert.equal(radarOf({ mix: {} }), null);
  assert.deepEqual(radarOf({ mix: { Blade: 10, Scholar: 5, Engineer: 0 } }), { blade: 1, scholar: 0.5, engineer: 0, summoner: 0 });
});

test("badges: this session's unlocks only, newest first, ≤ 4 then more", () => {
  const progress = { projects: [{ key: "k1", unlocks: [
    { id: "first_blood", at: 1, sessionID: "s1" }, { id: "night_owl", at: 5, sessionID: "s1" },
    { id: "marathon", at: 3, sessionID: "s2" }, { id: "flawless", at: 4, sessionID: "s1" },
    { id: "big_brain", at: 6, sessionID: "s1" }, { id: "speedrun", at: 7, sessionID: "s1" },
  ] }] };
  const b = badgesOf({ id: "s1", projectKey: "k1" }, progress);
  assert.deepEqual(b.list.map((x) => x.id), ["speedrun", "big_brain", "night_owl", "flawless"]);
  assert.equal(b.more, 1);
  assert.equal(b.list[0].name, "Speedrun");
  assert.ok(b.list[0].glyph);
  assert.deepEqual(badgesOf({ id: "s3", projectKey: "k1" }, progress), { list: [], more: 0 });
  assert.deepEqual(badgesOf({ id: "s1", projectKey: "k1" }, null), { list: [], more: 0 });
  assert.deepEqual(badgesOf({ id: "", projectKey: "k1" }, progress), { list: [], more: 0 });
});

test("pouch: full at 0 % used, dims at 80 %, hidden when unknown", () => {
  assert.equal(pouchOf(null), null);
  assert.deepEqual(pouchOf(0), { fill: 1, dim: false });
  assert.deepEqual(pouchOf(80), { fill: 0.2, dim: true });
  assert.deepEqual(pouchOf(130), { fill: 0, dim: true });
});

test("quotaOf takes the highest window utilisation; junk → null", () => {
  assert.equal(quotaOf({ five_hour: { utilization: 40 }, seven_day: { utilization: 72 }, x: null }), 72);
  assert.equal(quotaOf({}), null);
  assert.equal(quotaOf(null), null);
});

test("loot: gems per 100 lines (≤ 5), chest on commits, scrolls ≤ 3, text omits zeros", () => {
  assert.equal(lootOf(null), null);
  assert.deepEqual(lootOf({ loot: { commits: 2, lines: 250, tests: 7 } }), { gems: 3, chest: true, scrolls: 3, text: "+250 lines · 2 commits · 7 green tests" });
  assert.deepEqual(lootOf({ loot: { commits: 0, lines: 9000, tests: 0 } }), { gems: 5, chest: false, scrolls: 0, text: "+9000 lines" });
  assert.equal(lootOf({ loot: { commits: 0, lines: 0, tests: 0 } }), null);
  assert.deepEqual(lootOf({ loot: { commits: 1, lines: 0, tests: 1 } }).text, "1 commit · 1 green test");
});

test("motion budget: attack already spends 3 effects; still mode spends none", () => {
  assert.deepEqual(motionBudget("attack", 3, true), { auraPulse: false, gearDrift: false });
  assert.deepEqual(motionBudget("charge", 3, true), { auraPulse: true, gearDrift: true });
  assert.deepEqual(motionBudget("rest", 0, true), { auraPulse: false, gearDrift: true });
  assert.deepEqual(motionBudget("rest", 2, false), { auraPulse: false, gearDrift: false });
});

test("halo for archmage, spellblade, priestess; wings for the rest", () => {
  assert.deepEqual(["opus", "sonnet", "qwen", "haiku", "glm", "kimi"].map(wingsOf), ["halo", "halo", "halo", "wings", "wings", "wings"]);
});

// Minors round 2 (phase 6b review): Quest Clear's boss dissolve and chest were not
// counted, so aura + gear made four running effects. The cap is three.
test("motionBudget: Quest Clear counts its dissolve and chest", () => {
  const m = motionBudget("clear", 2, true);
  assert.equal(m.auraPulse, true);
  assert.equal(m.gearDrift, false);
});
