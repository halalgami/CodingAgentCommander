import { test } from "node:test";
import assert from "node:assert/strict";
import { loadRecents, pushRecent } from "./recents.js";

function fakeStorage() {
  const m = new Map();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, String(v)) };
}

test("empty storage yields empty list", () => {
  assert.deepEqual(loadRecents(fakeStorage()), []);
});

test("push + load round-trips, most recent first", () => {
  const s = fakeStorage();
  pushRecent({ folder: "/a", modelID: "m1" }, s);
  pushRecent({ folder: "/b", modelID: "m2" }, s);
  const r = loadRecents(s);
  assert.equal(r.length, 2);
  assert.equal(r[0].folder, "/b");
});

test("dedupes on folder+model, bumps to front", () => {
  const s = fakeStorage();
  pushRecent({ folder: "/a", modelID: "m1" }, s);
  pushRecent({ folder: "/b", modelID: "m1" }, s);
  pushRecent({ folder: "/a", modelID: "m1" }, s);
  const r = loadRecents(s);
  assert.equal(r.length, 2);
  assert.equal(r[0].folder, "/a");
});

test("caps at 8", () => {
  const s = fakeStorage();
  for (let i = 0; i < 12; i++) pushRecent({ folder: `/p${i}`, modelID: "m" }, s);
  assert.equal(loadRecents(s).length, 8);
});

test("corrupt storage yields empty list", () => {
  const s = fakeStorage();
  s.setItem("commander.recents.v1", "{not json");
  assert.deepEqual(loadRecents(s), []);
});
