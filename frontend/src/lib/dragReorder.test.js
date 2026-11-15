import { test } from "node:test";
import assert from "node:assert/strict";
import { dropTarget, DRAG_THRESHOLD } from "./dragReorder.js";

// ungrouped header 0-20, cards @1 20-60, @2 60-100; g_1 header 100-120, card @3 120-160; new 160-180
const R = [
  { kind: "header", key: "ungrouped", top: 0, bottom: 20 },
  { kind: "card", key: "ungrouped", windowID: "@1", top: 20, bottom: 60 },
  { kind: "card", key: "ungrouped", windowID: "@2", top: 60, bottom: 100 },
  { kind: "header", key: "g_1", top: 100, bottom: 120 },
  { kind: "card", key: "g_1", windowID: "@3", top: 120, bottom: 160 },
  { kind: "new", top: 160, bottom: 180 },
];

test("threshold is 4 px", () => assert.equal(DRAG_THRESHOLD, 4));

test("upper half of a card drops before it, lower half after", () => {
  assert.deepEqual(dropTarget(R, 65, "card", "@1"), { key: "ungrouped", index: 0 });
  assert.deepEqual(dropTarget(R, 95, "card", "@1"), { key: "ungrouped", index: 1 });
  assert.deepEqual(dropTarget(R, 25, "card", "@3"), { key: "ungrouped", index: 0 });
  assert.deepEqual(dropTarget(R, 55, "card", "@3"), { key: "ungrouped", index: 1 });
});

test("a header drops at the end of its group", () => {
  assert.deepEqual(dropTarget(R, 110, "card", "@1"), { key: "g_1", index: Infinity });
});

test("the new-group zone", () => {
  assert.deepEqual(dropTarget(R, 170, "card", "@1"), { newGroup: true });
});

test("outside the list clamps", () => {
  assert.deepEqual(dropTarget(R, -50, "card", "@3"), { key: "ungrouped", index: 0 });
  assert.deepEqual(dropTarget(R, 900, "card", "@1"), { newGroup: true });
  assert.equal(dropTarget([], 10, "card", "@1"), null);
});

test("group drags target header boundaries, never above ungrouped", () => {
  const G = [
    { kind: "header", key: "ungrouped", top: 0, bottom: 20 },
    { kind: "header", key: "g_1", top: 100, bottom: 120 },
    { kind: "header", key: "g_2", top: 200, bottom: 220 },
    { kind: "header", key: "g_3", top: 300, bottom: 320 },
  ];
  assert.deepEqual(dropTarget(G, -10, "group", "g_3"), { groupIndex: 0 });
  assert.deepEqual(dropTarget(G, 150, "group", "g_3"), { groupIndex: 1 });
  assert.deepEqual(dropTarget(G, 900, "group", "g_1"), { groupIndex: 2 });
});
