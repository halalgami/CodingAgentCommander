// frontend/src/lib/fleet/fleet.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { shipOf, shipName, encounterOf, actionOf, craftOf, wingOf, stateOf, veteranOf, fuelOf, spaceOf, fleetMotion, CLASSES } from "./fleet.js";

test("shipOf: class, tint and captain per model; pack name override", () => {
  const s = shipOf({ model: "claude-opus-5-5" });
  assert.deepEqual([s.id, s.cls, s.tint, s.captain, s.privateer], ["opus", "Flagship carrier", "#e0a33a", "Archmage", false]);
  assert.equal(shipOf({ model: "kimi-k2" }).cls, "Drone carrier");
  const p = shipOf({ model: "claude-opus-5-5" }, { files: {}, heroines: { opus: { name: "Layla", tint: "#112233" } } });
  assert.equal(p.captain, "Layla");
  assert.equal(p.tint, "#112233");
});

test("shipOf: unknown model is a Privateer on a borrowed hull", () => {
  const s = shipOf({ model: "gpt-x", id: "s9" });
  assert.equal(s.cls, "Privateer");
  assert.equal(s.privateer, true);
  assert.ok(Object.keys(CLASSES).includes(s.id));
});

test("shipName is stable per session and prefixed", () => {
  const a = shipName({ id: "abc" });
  assert.match(a, /^ACS [A-Z][a-z]+$/);
  assert.equal(shipName({ id: "abc" }), a);
  const names = new Set(Array.from({ length: 40 }, (_, i) => shipName({ id: "s" + i })));
  assert.ok(names.size > 10);
});

test("encounterOf follows Raid's boss rules", () => {
  assert.deepEqual(encounterOf({ title: "fix crash" }, null), { kind: "swarm", name: "Alien Swarm", level: 1 });
  assert.equal(encounterOf({ title: "refactor legacy" }, null).kind, "derelict");
  assert.equal(encounterOf({ title: "deploy ci" }, null).kind, "storm");
  assert.equal(encounterOf({ title: "sql schema" }, null).kind, "anomaly");
  assert.equal(encounterOf({ title: "design spec" }, null).kind, "gate");
  assert.ok(encounterOf({ title: "fix" }, { code: 50000, complexity: 800 }).level > 1);
});

test("actionOf per tool", () => {
  assert.equal(actionOf("Read"), "sweep");
  assert.equal(actionOf("Grep"), "sweep");
  assert.equal(actionOf("Edit"), "beam");
  assert.equal(actionOf("Write"), "beam");
  assert.equal(actionOf("Bash"), "salvo");
  assert.equal(actionOf("WebFetch"), "scan");
  assert.equal(actionOf("Task"), "launch");
  assert.equal(actionOf("Agent"), "launch");
  assert.equal(actionOf(undefined), "idle");
});

test("craftOf by agent type; wingOf caps at 4 and counts the rest", () => {
  assert.equal(craftOf({ type: "Explore" }), "scout");
  assert.equal(craftOf({ type: "Plan" }), "pathfinder");
  assert.equal(craftOf({ type: "code-reviewer" }), "sentinel");
  assert.equal(craftOf({ type: "general-purpose" }), "tug");
  const crew = Array.from({ length: 6 }, (_, i) => ({ id: "a" + i, type: i ? "general-purpose" : "Explore", depth: 1 }));
  crew.push({ id: "deep", type: "Explore", depth: 3 });
  const w = wingOf(crew);
  assert.equal(w.craft.length, 4);
  assert.equal(w.craft[0].kind, "scout");
  assert.equal(w.more, 3);
  assert.deepEqual(wingOf([]), { craft: [], more: 0 });
});

test("stateOf, veteranOf, fuelOf, spaceOf", () => {
  assert.deepEqual(["charge", "attack", "orders", "ko", "clear", "rest"].map(stateOf), ["engage", "engage", "await", "damaged", "jump", "docked"]);
  assert.deepEqual(veteranOf(null), []);
  assert.deepEqual(veteranOf({ tier: 1 }), ["glow"]);
  assert.deepEqual(veteranOf({ tier: 2 }), ["glow", "escort"]);
  assert.deepEqual(veteranOf({ tier: 9 }), ["glow", "escort", "trim"]);
  assert.equal(fuelOf({ ctxPct: 42 }), 0.42);
  assert.equal(fuelOf({ ctxPct: 130 }), 1);
  assert.equal(fuelOf({}), null);
  assert.deepEqual([7, 13, 22, 3].map(spaceOf), ["deep", "nebula", "planet", "planet"]);
});

test("fleetMotion keeps the panel at 3 effects", () => {
  assert.deepEqual(fleetMotion("engage", ["glow", "escort"], true), { glowPulse: true, escortDrift: false });
  assert.deepEqual(fleetMotion("docked", ["glow", "escort"], true), { glowPulse: true, escortDrift: true });
  assert.deepEqual(fleetMotion("engage", ["glow"], false), { glowPulse: false, escortDrift: false });
  assert.deepEqual(fleetMotion("docked", [], true), { glowPulse: false, escortDrift: false });
});

// Minors round 2 (Fleet Ops review): the Babylon 5 jump (vortex out, vortex in,
// flash and the ship's flight) is the whole budget; glow and escort stay still.
test("fleetMotion: the jump uses the whole 3-effect budget", () => {
  assert.deepEqual(fleetMotion("jump", ["glow", "escort"], true), { glowPulse: false, escortDrift: false });
});

// Minors round 2 (Fleet Ops review): coverage the review asked for.
test("wingOf: depth-3 agents are counted in +N, never drawn", () => {
  const crew = [{ id: "a", depth: 1 }, { id: "b", depth: 2 }, { id: "c", depth: 3 }, { id: "d", depth: 3 }];
  const w = wingOf(crew);
  assert.deepEqual(w.craft.map((c) => c.id), ["a", "b"]);
  assert.equal(w.more, 2);
});

test("shipArt: a damaged ship falls back to the intact sprite", async () => {
  const { shipArt } = await import("./art.js");
  const s = shipArt("opus", true);
  assert.ok(Array.isArray(s) && s[0].includes("ship-opus-damaged") && s[1].includes("ship-opus.") && !s[1].includes("damaged"));
});
