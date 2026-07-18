import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULTS, loadPrefs, savePrefs } from "./prefsData.js";

function fakeStorage() {
  const m = new Map();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, String(v)) };
}

test("empty storage yields defaults", () => {
  assert.deepEqual(loadPrefs(fakeStorage()), DEFAULTS);
});

test("save + load round-trips", () => {
  const s = fakeStorage();
  savePrefs({ ...DEFAULTS, fontSize: 16 }, s);
  assert.equal(loadPrefs(s).fontSize, 16);
});

test("corrupt json yields defaults", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v2", "{nope");
  assert.deepEqual(loadPrefs(s), DEFAULTS);
});

test("wrong-typed and unknown keys are dropped", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v2", JSON.stringify({ fontSize: "huge", evil: 1, uiScale: 110 }));
  const p = loadPrefs(s);
  assert.equal(p.fontSize, DEFAULTS.fontSize);
  assert.equal(p.uiScale, 110);
  assert.equal("evil" in p, false);
});

test("defaults are the spec values", () => {
  assert.deepEqual(DEFAULTS, { fontSize: 13, scrollback: 5000, maxCols: 0, uiScale: 100, sidebarW: 300, rcAutoEnable: false });
});

test("v1 payload migrates: default 120 cap becomes unlimited", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v1", JSON.stringify({ ...DEFAULTS, maxCols: 120, fontSize: 15 }));
  const p = loadPrefs(s);
  assert.equal(p.maxCols, 0);
  assert.equal(p.fontSize, 15);
});

test("v1 payload keeps a non-default cap", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v1", JSON.stringify({ ...DEFAULTS, maxCols: 140 }));
  assert.equal(loadPrefs(s).maxCols, 140);
});

test("v2 explicit 120 cap persists", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v2", JSON.stringify({ ...DEFAULTS, maxCols: 120 }));
  assert.equal(loadPrefs(s).maxCols, 120);
});
