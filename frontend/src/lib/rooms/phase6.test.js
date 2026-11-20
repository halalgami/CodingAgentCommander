import { test } from "node:test";
import assert from "node:assert/strict";
import { baseName, fmtSize, authorLine, canSend } from "./rooms.js";

test("baseName handles both separators", () => {
  assert.equal(baseName("/Users/x/shot.png"), "shot.png");
  assert.equal(baseName("C:\\Users\\x\\notes.txt"), "notes.txt");
});

test("fmtSize", () => {
  assert.equal(fmtSize(512), "512 B");
  assert.equal(fmtSize(2048), "2.0 KB");
  assert.equal(fmtSize(3 * 1024 * 1024), "3.0 MB");
});

test("authorLine shows the user's name beside the machine", () => {
  assert.equal(authorLine({ author: "mini", user: "Hasan" }), "Hasan · mini");
  assert.equal(authorLine({ author: "mini" }), "mini");
});

test("canSend: text or files, under the byte cap, not busy", () => {
  assert.equal(canSend("", [], 0, false), false);
  assert.equal(canSend("", ["/a"], 0, false), true);
  assert.equal(canSend("hi", [], 17000, false), false);
  assert.equal(canSend("hi", [], 10, true), false);
});

import { groupRoomId } from "./rooms.js";
test("groupRoomId: one room shared by every session in the group", () => {
  const roomOf = (c) => ({ "/a": "r1", "/a2": "r1", "/b": "r2" })[c] || "";
  assert.equal(groupRoomId(["/a", "/a2"], roomOf), "r1");
  assert.equal(groupRoomId(["/a", "/b"], roomOf), "");
  assert.equal(groupRoomId(["/a", "/x"], roomOf), "");
  assert.equal(groupRoomId([], roomOf), "");
});
