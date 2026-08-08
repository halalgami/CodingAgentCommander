import { test } from "node:test";
import assert from "node:assert/strict";
import { foldRecents, readLegacyRecents, migrateHistoryOnce } from "./history.js";

function fakeStorage(initial = {}) {
  const m = new Map(Object.entries(initial));
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
    removeItem: (k) => m.delete(k),
    _has: (k) => m.has(k),
  };
}

test("foldRecents collapses folder+model pairs into one row per folder", () => {
  const folded = foldRecents([
    { folder: "/a", modelID: "m1", ts: 100 },
    { folder: "/a", modelID: "m2", ts: 300 }, // newer -> wins lastModelID
    { folder: "/b", modelID: "m3", ts: 200 },
  ]);
  assert.equal(folded.length, 2);
  const a = folded.find((e) => e.folder === "/a");
  assert.equal(a.openCount, 2);
  assert.equal(a.lastModelID, "m2");
  assert.equal(a.lastOpened, 300);
  assert.equal(a.pinned, false);
});

test("foldRecents skips malformed rows", () => {
  const folded = foldRecents([null, { modelID: "x" }, { folder: "/ok", modelID: "m", ts: 1 }]);
  assert.equal(folded.length, 1);
  assert.equal(folded[0].folder, "/ok");
});

test("readLegacyRecents parses, tolerates corrupt", () => {
  assert.deepEqual(readLegacyRecents(fakeStorage()), []);
  assert.deepEqual(readLegacyRecents(fakeStorage({ "commander.recents.v1": "{bad" })), []);
  const good = fakeStorage({ "commander.recents.v1": JSON.stringify([{ folder: "/a", modelID: "m", ts: 1 }]) });
  assert.equal(readLegacyRecents(good).length, 1);
});

test("migrateHistoryOnce imports folded entries then clears the legacy key", async () => {
  const storage = fakeStorage({
    "commander.recents.v1": JSON.stringify([
      { folder: "/a", modelID: "m1", ts: 1 },
      { folder: "/a", modelID: "m2", ts: 2 },
    ]),
  });
  let imported = null;
  await migrateHistoryOnce(storage, async (entries) => { imported = entries; });
  assert.ok(imported, "import fn must be called");
  assert.equal(imported.length, 1);
  assert.equal(imported[0].folder, "/a");
  assert.equal(storage._has("commander.recents.v1"), false, "legacy key cleared");
});

test("migrateHistoryOnce with no legacy data still clears key and does not import", async () => {
  const storage = fakeStorage();
  let called = false;
  await migrateHistoryOnce(storage, async () => { called = true; });
  assert.equal(called, false);
});

test("migrateHistoryOnce keeps the legacy key if import throws (retry next run)", async () => {
  const storage = fakeStorage({
    "commander.recents.v1": JSON.stringify([{ folder: "/a", modelID: "m", ts: 1 }]),
  });
  await migrateHistoryOnce(storage, async () => { throw new Error("backend down"); });
  assert.equal(storage._has("commander.recents.v1"), true, "must not clear on failed import");
});
