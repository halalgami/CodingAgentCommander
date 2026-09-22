// The wizard's decisions, tested away from runes, Wails and the DOM.
//
// The rule under test throughout: the run lives in Go, and the phase is
// DERIVED from it. Storing a phase independently is what makes a wizard
// reopened mid-run show step 1 with a "Generate" button over a run that is
// already spending money.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  slotKey, phaseFor, hasArt, selectedSlotsFrom, defaultPicks, conditionsFor,
  nextModel, blockerFor, statusText, costLine,
} from "./packgen-logic.js";

const SLOT_DEFAULTS = [
  { slot: "idle", staging: "idle default" },
  { slot: "working", staging: "working default" },
  { slot: "bored", staging: "bored default" },
];
const doneItem = (slot) => ({ slot, when: "", status: "done", preview: "abc" });

// --- derived phase ---------------------------------------------------------

test("phase follows the user's step while no run exists", () => {
  assert.equal(phaseFor(null, "base"), "base");
  assert.equal(phaseFor(null, "slots"), "slots");
});

test("a running run forces the generate phase whatever the step says", () => {
  const run = { id: "r1", status: "running", items: [], saved: false };
  assert.equal(phaseFor(run, "base"), "generate");
  assert.equal(phaseFor(run, "slots"), "generate");
});

test("a finished run with unsaved art forces review", () => {
  const run = { id: "r1", status: "done", saved: false, items: [doneItem("idle")] };
  assert.equal(phaseFor(run, "base"), "review");
});

test("a cancelled run that produced art still needs a decision", () => {
  const run = { id: "r1", status: "cancelled", saved: false, items: [doneItem("idle")] };
  assert.equal(phaseFor(run, "base"), "review",
    "cancelled images are paid for and must be savable");
});

test("a saved run releases the wizard back to the form", () => {
  const run = { id: "r1", status: "done", saved: true, items: [doneItem("idle")] };
  assert.equal(phaseFor(run, "base"), "base");
});

test("a run that produced nothing does not trap the user in review", () => {
  const run = {
    id: "r1", status: "failed", saved: false,
    items: [{ slot: "idle", when: "", status: "failed", error: "boom" }],
  };
  assert.equal(phaseFor(run, "base"), "base",
    "there is nothing to review, so the form must be reachable");
});

// A stale step must never surface a run phase with no run behind it — the grid
// would render zero cards above a Save button.
test("a leftover run-phase step falls back to the form", () => {
  assert.equal(phaseFor(null, "review"), "base");
  assert.equal(phaseFor(null, "generate"), "base");
});

test("hasArt counts only images that actually landed", () => {
  assert.equal(hasArt({ items: [{ status: "failed" }, { status: "skipped" }] }), false);
  assert.equal(hasArt({ items: [{ status: "failed" }, { status: "done" }] }), true);
  assert.equal(hasArt(null), false);
});

// --- slot selection --------------------------------------------------------

test("defaultPicks turns on every base slot with its default staging", () => {
  const picked = selectedSlotsFrom(defaultPicks(SLOT_DEFAULTS));
  assert.equal(picked.length, 3);
  assert.deepEqual(picked.map((s) => s.slot).sort(), ["bored", "idle", "working"]);
  assert.equal(picked.find((s) => s.slot === "idle").staging, "idle default");
  assert.ok(picked.every((s) => s.when === ""), "conditions start off");
});

test("a conditioned variant is a separate pick keyed by slot and condition", () => {
  const picks = defaultPicks(SLOT_DEFAULTS);
  picks["bored|lateNight"] = { on: true, staging: "bored default" };
  const picked = selectedSlotsFrom(picks);
  assert.equal(picked.length, 4);
  const night = picked.find((s) => s.when === "lateNight");
  assert.equal(night.slot, "bored");
});

test("unticking removes a slot from the request", () => {
  const picks = defaultPicks(SLOT_DEFAULTS);
  picks.working.on = false;
  assert.ok(!selectedSlotsFrom(picks).some((s) => s.slot === "working"));
});

test("a conditioned pick carries its own staging, not the base slot's", () => {
  const picks = defaultPicks(SLOT_DEFAULTS);
  picks["bored|lateNight"] = { on: true, staging: "she is asleep" };
  const picked = selectedSlotsFrom(picks);
  assert.equal(picked.find((s) => s.when === "lateNight").staging, "she is asleep");
  assert.equal(picked.find((s) => s.slot === "bored" && !s.when).staging, "bored default");
});

test("slotKey distinguishes a base slot from its conditioned variant", () => {
  assert.equal(slotKey("bored", ""), "bored");
  assert.equal(slotKey("bored", "lateNight"), "bored|lateNight");
  assert.notEqual(slotKey("bored", ""), slotKey("bored", "lateNight"));
});

// --- the condition matrix comes from Go ------------------------------------

test("only conditions Go says can display in a slot are offered", () => {
  const conds = [
    { name: "lateNight", class: "ambient", slots: ["idle", "bored"] },
    { name: "marathon", class: "ambient", slots: ["working"] },
  ];
  assert.deepEqual(conditionsFor(conds, "bored").map((c) => c.name), ["lateNight"]);
  assert.deepEqual(conditionsFor(conds, "working").map((c) => c.name), ["marathon"]);
  assert.deepEqual(conditionsFor(conds, "done").map((c) => c.name), [],
    "a slot with no compatible condition offers none");
});

// --- provider and model ----------------------------------------------------

const PROVIDERS = [
  { id: "fal", hasKey: true, models: [{ id: "fal/a" }, { id: "fal/b" }] },
  { id: "other", hasKey: false, models: [{ id: "other/x" }] },
];

test("switching provider moves the model to one that provider offers", () => {
  assert.equal(nextModel(PROVIDERS, "fal", ""), "fal/a");
  // A model from the previous provider must not survive the switch — Go would
  // reject it, but only after a whole run was configured.
  assert.equal(nextModel(PROVIDERS, "other", "fal/a"), "other/x");
});

test("a still-valid model choice is kept", () => {
  assert.equal(nextModel(PROVIDERS, "fal", "fal/b"), "fal/b");
});

test("a provider with no models yields an empty string, never undefined", () => {
  const got = nextModel([{ id: "empty", hasKey: true, models: [] }], "empty", "x");
  assert.equal(got, "", "undefined reaches Go as the string 'undefined'");
  assert.equal(nextModel(PROVIDERS, "nope", "x"), "");
});

// --- the blocker -----------------------------------------------------------

const OK = {
  name: "Wafa",
  base: { path: "/tmp/a.png" },
  provider: { id: "fal", hasKey: true },
  model: "fal/a",
  picks: defaultPicks(SLOT_DEFAULTS),
  run: null,
  conditions: [
    { name: "lateNight", class: "ambient", slots: ["bored"] },
    { name: "weekend", class: "ambient", slots: ["bored"] },
    { name: "century", class: "event", slots: ["done"] },
  ],
};

test("a complete form has no blocker", () => {
  assert.equal(blockerFor(OK), "");
});

test("every missing piece names itself", () => {
  const cases = [
    [{ name: "  " }, /name/i],
    [{ base: null }, /portrait/i],
    [{ provider: null }, /provider/i],
    [{ provider: { id: "fal", hasKey: false } }, /API key/i],
    [{ model: "" }, /model/i],
    [{ picks: {} }, /at least one scene/i],
  ];
  for (const [patch, want] of cases) {
    const got = blockerFor({ ...OK, ...patch });
    assert.match(got, want, `patch ${JSON.stringify(patch)} gave ${JSON.stringify(got)}`);
  }
});

// Mirrors validatePackGenSlots. Catching it here means the user is told while
// choosing rather than refused after pressing Generate.
test("idle is required, and a conditioned idle does not satisfy it", () => {
  const noIdle = { working: { on: true, staging: "" } };
  assert.match(blockerFor({ ...OK, picks: noIdle }), /idle scene is required/i);

  const condIdle = { "idle|lateNight": { on: true, staging: "" } };
  assert.match(blockerFor({ ...OK, picks: condIdle }), /idle scene is required/i);
});

test("two ambient conditions on one slot are refused before spending", () => {
  const picks = {
    ...defaultPicks(SLOT_DEFAULTS),
    "bored|lateNight": { on: true, staging: "" },
    "bored|weekend": { on: true, staging: "" },
  };
  const got = blockerFor({ ...OK, picks });
  assert.match(got, /both be true at once/i);
  assert.match(got, /lateNight, weekend/, "the message must name them, in a stable order");
});

test("one ambient plus one event on the same slot is fine", () => {
  const picks = {
    ...defaultPicks(SLOT_DEFAULTS),
    done: { on: true, staging: "" },
    "done|century": { on: true, staging: "" },   // event
  };
  assert.equal(blockerFor({ ...OK, picks }), "");
});

test("a run in flight or holding unsaved art blocks a new one", () => {
  const running = { status: "running", items: [], saved: false };
  assert.match(blockerFor({ ...OK, run: running }), /already generating/i);

  const unsaved = { status: "done", saved: false, items: [doneItem("idle")] };
  assert.match(blockerFor({ ...OK, run: unsaved }), /save or discard/i);

  const saved = { status: "done", saved: true, items: [doneItem("idle")] };
  assert.equal(blockerFor({ ...OK, run: saved }), "");
});

// --- presentation ----------------------------------------------------------

test("statusText explains every non-final state", () => {
  assert.equal(statusText({ status: "done" }), "");
  assert.equal(statusText({ status: "pending" }), "queued");
  assert.equal(statusText({ status: "running" }), "generating…");
  assert.equal(statusText({ status: "skipped" }), "skipped");
  assert.equal(statusText({ status: "failed", error: "boom" }), "boom");
  // A failure with no message must still read as a failure, not as blank.
  assert.equal(statusText({ status: "failed" }), "failed");
});

// A stale or absent price rendered as $0.00 reads as free, which is the one
// thing a cost line must never say.
test("an unknown price shows a count and no figure", () => {
  const got = costLine(6, { known: false, usd: 0, note: "prices as of 2026-08-30" });
  assert.equal(got.price, "");
  assert.equal(got.images, "6 images");
  assert.match(got.note, /unknown/i);
});

test("a known price is shown with its disclosure", () => {
  const got = costLine(6, { known: true, usd: 0.48, note: "prices as of 2026-08-30" });
  assert.equal(got.price, "$0.48");
  assert.equal(got.images, "6 images");
  assert.match(got.note, /prices as of/);
});

test("one image is not pluralised", () => {
  assert.equal(costLine(1, { known: true, usd: 0.08 }).images, "1 image");
});

import {
  packModel, addableScenes, selectedAddSlots, runHoldsArt,
} from "./packgen-logic.js";

const CONDITIONS = [
  { name: "night", class: "ambient", slots: ["idle", "bored"] },
  { name: "rainy", class: "ambient", slots: ["idle"] },
];
const browseOf = (variants) => ({ name: "P", path: "/p", hasBase: true, variants });

// --- packModel -------------------------------------------------------------

test("packModel returns the most common recorded model", () => {
  const b = browseOf([
    { slot: "idle", when: "", model: "m1" },
    { slot: "bored", when: "", model: "m2" },
    { slot: "working", when: "", model: "m2" },
  ]);
  assert.equal(packModel(b), "m2");
});

test("packModel skips variants with no model and returns '' when none", () => {
  assert.equal(packModel(browseOf([{ slot: "idle", when: "" }])), "");
  assert.equal(packModel(browseOf([])), "");
  assert.equal(packModel(null), "");
});

test("packModel breaks ties toward the first model seen", () => {
  const b = browseOf([
    { slot: "idle", when: "", model: "a" },
    { slot: "bored", when: "", model: "b" },
  ]);
  assert.equal(packModel(b), "a");
});

// --- addableScenes ---------------------------------------------------------

test("addableScenes marks present combos and lists conditions", () => {
  const b = browseOf([{ slot: "idle", when: "", model: "m1" }]);
  const rows = addableScenes(SLOT_DEFAULTS, CONDITIONS, b);
  const idle = rows.find((r) => r.slot === "idle" && r.when === "");
  const idleNight = rows.find((r) => r.slot === "idle" && r.when === "night");
  const boredBase = rows.find((r) => r.slot === "bored" && r.when === "");
  assert.equal(idle.present, true, "idle base is in the pack");
  assert.equal(idleNight.present, false, "idle·night is not");
  assert.equal(boredBase.present, false, "bored base is not");
  assert.equal(idleNight.label, "idle · night");
});

test("addableScenes marks a slot|when present even when the saved variant has a mood", () => {
  // Load-bearing: the present key is slot|when, broader than record()'s
  // slot|when|mood. A moody saved variant must block the whole slot|when.
  const b = browseOf([
    { slot: "idle", when: "", model: "m1" },
    { slot: "bored", when: "night", mood: "sad", model: "m1" },
  ]);
  const rows = addableScenes(SLOT_DEFAULTS, CONDITIONS, b);
  const boredNight = rows.find((r) => r.slot === "bored" && r.when === "night");
  assert.equal(boredNight.present, true, "bored·night present at some mood → not addable");
});

// --- selectedAddSlots ------------------------------------------------------

test("selectedAddSlots emits ticked picks in PackGenSlot shape", () => {
  const picks = {
    "bored|night": { on: true, staging: "s1" },
    "idle|rainy": { on: false, staging: "s2" },
  };
  assert.deepEqual(selectedAddSlots(picks, new Set()), [
    { slot: "bored", when: "night", staging: "s1" },
  ]);
});

test("selectedAddSlots never emits a combo present at any mood", () => {
  const picks = { "bored|night": { on: true, staging: "s1" } };
  const present = new Set(["bored|night"]);
  assert.deepEqual(selectedAddSlots(picks, present), []);
});

// --- runHoldsArt -----------------------------------------------------------

test("runHoldsArt trusts the run's own hasArt when present", () => {
  assert.equal(runHoldsArt({ status: "done", saved: false, hasArt: true, items: [] }), true);
  assert.equal(runHoldsArt({ status: "done", saved: false, hasArt: false, items: [doneItem("idle")] }), false);
});

test("runHoldsArt falls back to done-item count when hasArt is absent", () => {
  assert.equal(runHoldsArt({ status: "done", saved: false, items: [doneItem("idle")] }), true);
  assert.equal(runHoldsArt({ status: "done", saved: false, items: [] }), false);
  assert.equal(runHoldsArt(null), false);
});

// --- phase/blocker now consult runHoldsArt ---------------------------------

test("a failed add (done, hasArt true, no done items) still reaches review", () => {
  const run = { id: "r", status: "done", saved: false, hasArt: true, items: [{ slot: "bored", when: "night", status: "failed" }] };
  assert.equal(phaseFor(run, "base"), "review");
});

test("blocker points at the held art when a run holds unsaved art via hasArt", () => {
  const msg = blockerFor({
    name: "P", base: { path: "x" }, provider: { id: "fal", hasKey: true }, model: "m",
    picks: { idle: { on: true } },
    run: { status: "done", saved: false, hasArt: true, items: [] },
    conditions: [],
  });
  assert.match(msg, /save or discard/i);
});
