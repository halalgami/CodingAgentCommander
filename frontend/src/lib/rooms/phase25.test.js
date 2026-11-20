import { test } from "node:test";
import assert from "node:assert/strict";
import { liveLine, byteLen, inboxActions } from "./rooms.js";

test("liveLine shows label, state, title, branch@sha and listening", () => {
  assert.equal(liveLine({ label: "api", state: "working", title: "Fix login", branch: "main", sha: "abcdef123", dirty: true, listening: "dev:42" }),
    "api · working · Fix login · main@abcdef1* · listening on dev:42");
  assert.equal(liveLine({}), "session · idle");
});

test("byteLen counts UTF-8 bytes", () => {
  assert.equal(byteLen("aé"), 3);
  assert.equal(byteLen(""), 0);
});

test("inboxActions: deliver only with a waiting session, claim for handoffs", () => {
  assert.deepEqual(inboxActions({ kind: "note" }), ["start", "reply", "dismiss"]);
  assert.deepEqual(inboxActions({ kind: "handoff" }, { waiting: true, sessions: 2 }), ["deliver", "start", "send", "claim", "reply", "dismiss"]);
  assert.deepEqual(inboxActions({ kind: "question" }, { folder: false }), ["claim", "reply", "dismiss"]);
});

import { promptOf, cleanText } from "./rooms.js";
test("copied prompts carry no terminal control characters", () => {
  const p = promptOf({ author: "m\x1b[2J", kind: "note", text: "hi\x1b[201~\rrm -rf ~\x07\x7f\u009b", fields: { goal: "a\nb" } }, "R");
  assert.equal(/[\u0000-\u0009\u000b-\u001f\u007f-\u009f]/.test(p), false);
  assert.equal(p, "[Room R · from m[2J · note]\nhi[201~rm -rf ~\ngoal: a b");
  assert.equal(cleanText("a\r\nb"), "a\nb");
});
