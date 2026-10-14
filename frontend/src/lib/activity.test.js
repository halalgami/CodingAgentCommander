import { test } from "node:test";
import assert from "node:assert/strict";
import { activityText } from "./activity.js";

test("names the tool and target", () => {
  assert.equal(activityText({ activity: "tool", tool: "Edit", target: "app.go" }), "Edit app.go");
  assert.equal(activityText({ activity: "tool", tool: "Bash", target: "" }), "Bash");
});

test("covers every state", () => {
  assert.equal(activityText({ activity: "thinking" }), "Thinking…");
  assert.equal(activityText({ activity: "permission" }), "Needs your approval");
  assert.equal(activityText({ activity: "finished", unread: true }), "Finished — your turn");
  assert.equal(activityText({ activity: "finished", unread: false }), "Finished");
  assert.equal(activityText({ activity: "idle" }), "Idle");
  assert.equal(activityText({ activity: "error" }), "Stopped on an API error");
});

test("is empty without data", () => {
  assert.equal(activityText(null), "");
  assert.equal(activityText({ activity: "" }), "");
});

test("flags hotspot files", () => {
  assert.equal(activityText({ activity: "tool", tool: "Edit", target: "app.go", hotspot: true }), "Edit app.go ⚠ hotspot");
  assert.equal(activityText({ activity: "thinking", hotspot: true }), "Thinking…");
});

test("crew activity texts", () => {
  assert.equal(activityText({ activity: "tool", tool: "Agent", target: "x", delegating: 3 }), "Waiting on 3 agents");
  assert.equal(activityText({ activity: "tool", tool: "Agent", target: "x", delegating: 1 }), "Waiting on 1 agent");
  assert.equal(activityText({ activity: "finished", unread: true, crewOut: 2 }), "2 agents out");
  assert.equal(activityText({ activity: "idle", crewOut: 1 }), "1 agent out");
  assert.equal(activityText({ activity: "tool", tool: "Edit", target: "a.go", hotspot: true }), "Edit a.go ⚠ hotspot");
});
