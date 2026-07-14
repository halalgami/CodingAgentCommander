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
  s.setItem("commander.prefs.v1", "{nope");
  assert.deepEqual(loadPrefs(s), DEFAULTS);
});

test("wrong-typed and unknown keys are dropped", () => {
  const s = fakeStorage();
  s.setItem("commander.prefs.v1", JSON.stringify({ fontSize: "huge", evil: 1, uiScale: 110 }));
  const p = loadPrefs(s);
  assert.equal(p.fontSize, DEFAULTS.fontSize);
  assert.equal(p.uiScale, 110);
  assert.equal("evil" in p, false);
});

test("defaults are the spec values", () => {
  assert.deepEqual(DEFAULTS, { fontSize: 13, scrollback: 5000, maxCols: 120, uiScale: 100, sidebarW: 300, rcAutoEnable: false });
});
