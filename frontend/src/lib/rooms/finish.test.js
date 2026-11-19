import { test } from "node:test";
import assert from "node:assert/strict";
import { promptOf, cmdrCode, pasteVerdict, handoffBranch } from "./rooms.js";

test("promptOf wraps a message the way Send to session will", () => {
  const m = { author: "MacBook Pro", kind: "handoff", text: "Finish the rename", fields: { goal: "ship", branch: "", verify: "go test" } };
  assert.equal(promptOf(m, "Commander"), "[Room Commander · from MacBook Pro · handoff]\nFinish the rename\ngoal: ship\nverify: go test");
  assert.equal(promptOf({ author: "a", kind: "note", text: "hi" }, "R"), "[Room R · from a · note]\nhi");
});

test("cmdrCode accepts a pasted code and nothing else", () => {
  const c = "CMDR-" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".repeat(2);
  assert.equal(cmdrCode("  " + c + "\n"), c);
  assert.equal(cmdrCode(c.slice(0, 20) + "\n" + c.slice(20)), c); // wrapped by a chat app
  assert.equal(cmdrCode("CMDR-abc"), "");
  assert.equal(cmdrCode("see CMDR-" + "A".repeat(30)), "");
  assert.equal(cmdrCode(""), "");
});

const el = (sel) => ({ tagName: sel.tag, isContentEditable: !!sel.ce, closest: (q) => (q === ".xterm" && sel.xterm ? {} : null) });

test("pasteVerdict never hijacks the terminal or a text field", () => {
  const c = "CMDR-" + "A".repeat(40);
  assert.equal(pasteVerdict(el({ tag: "TEXTAREA", xterm: true }), c), "terminal");
  assert.equal(pasteVerdict(el({ tag: "INPUT" }), c), "ignore");
  assert.equal(pasteVerdict(el({ tag: "TEXTAREA" }), c), "ignore");
  assert.equal(pasteVerdict(el({ tag: "DIV", ce: true }), c), "ignore");
  assert.equal(pasteVerdict(el({ tag: "BODY" }), c), "join");
  assert.equal(pasteVerdict(null, c), "join");
  assert.equal(pasteVerdict(el({ tag: "BODY" }), "hello"), "ignore");
});

test("handoffBranch formats branch and short sha", () => {
  assert.equal(handoffBranch({ branch: "feat/x", sha: "0123456789abcdef0123456789abcdef01234567" }), "feat/x @ 0123456");
  assert.equal(handoffBranch({ branch: "HEAD", sha: "0123456789abcdef" }), "0123456");
  assert.equal(handoffBranch(null), "");
});
