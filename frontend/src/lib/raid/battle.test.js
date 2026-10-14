// frontend/src/lib/raid/battle.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { hitsOf, lastHitAt, battleOf } from "./battle.js";

const NOW = 50_000_000;
const ev = (secAgo, kind, extra = {}) => ({ session: "s1", at: NOW - secAgo * 1000, kind, ...extra });
const row = (extra = {}) => ({ id: "s1", activity: "tool", sinceMs: NOW - 5000, ctxPct: 40, unread: false, crewOut: 0, compactedMs: 0, ...extra });

test("hits count tool marks in the current run only", () => {
  // old run ended with the assistant's answer (think) > 2 min before the new prompt's think + 3 tools
  const evs = [ev(400, "edit"), ev(390, "shell"), ev(385, "think"), ev(60, "think"), ev(50, "read"), ev(40, "edit"), ev(30, "shell")];
  assert.equal(hitsOf(evs), 3);
});

// Review fix: a long tool (a 5-minute test run) is still the same fight — the
// boss must not heal back to full mid-task.
test("a long-running tool does not start a new run", () => {
  const evs = [ev(400, "edit"), ev(395, "shell"), ev(60, "think"), ev(50, "edit")];
  assert.equal(hitsOf(evs), 3);
});

test("a permission pause does not end the run; agent marks never count", () => {
  const evs = [ev(500, "edit"), ev(495, "wait", { label: "start" }), ev(200, "wait", { label: "end" }), ev(195, "edit"),
    ev(190, "read", { agent: "a1" })];
  assert.equal(hitsOf(evs), 2);
  assert.equal(hitsOf([]), 0);
  assert.equal(lastHitAt(evs), NOW - 195000);
  assert.equal(lastHitAt([ev(5, "think")]), 0);
});

test("HP drains 7 per hit and floors at 5", () => {
  assert.equal(battleOf(row(), [], NOW).hp, 100);
  assert.equal(battleOf(row(), [ev(3, "edit"), ev(2, "edit")], NOW).hp, 86);
  const many = Array.from({ length: 40 }, (_, i) => ev(40 - i, "edit"));
  assert.equal(battleOf(row(), many, NOW).hp, 5);
});

test("state, pose and boss per row activity", () => {
  const s = (r) => { const b = battleOf(row(r), [], NOW); return [b.state, b.pose, b.boss]; };
  assert.deepEqual(s({ activity: "thinking" }), ["charge", "stance", "present"]);
  assert.deepEqual(s({ activity: "tool" }), ["attack", "attack", "present"]);
  assert.deepEqual(s({ activity: "permission" }), ["orders", "await", "present"]);
  assert.deepEqual(s({ activity: "finished", unread: true }), ["clear", "victory", "dissolve"]);
  assert.deepEqual(s({ activity: "error" }), ["ko", "hurt", "present"]);
  assert.deepEqual(s({ activity: "finished", unread: false }), ["rest", "stance", "absent"]);
  assert.deepEqual(s({ activity: "idle" }), ["rest", "stance", "absent"]);
  assert.deepEqual(s({ activity: "" }), ["rest", "stance", "absent"]);
  assert.deepEqual(s({ activity: "finished", crewOut: 2 }), ["charge", "stance", "present"]); // waiting on allies
});

test("enraged after 10 minutes of work only", () => {
  assert.equal(battleOf(row({ sinceMs: NOW - 11 * 60000 }), [], NOW).enraged, true);
  assert.equal(battleOf(row({ sinceMs: NOW - 9 * 60000 }), [], NOW).enraged, false);
  assert.equal(battleOf(row({ activity: "permission", sinceMs: NOW - 30 * 60000 }), [], NOW).enraged, false);
  assert.equal(battleOf(row({ sinceMs: 0 }), [], NOW).enraged, false);
});

test("patience stages while waiting for orders", () => {
  const p = (min) => battleOf(row({ activity: "permission", sinceMs: NOW - min * 60000 }), [], NOW).patience;
  assert.equal(p(1), "tapping");
  assert.equal(p(5), "sitting");
  assert.equal(p(10), "asleep");
  assert.equal(battleOf(row(), [], NOW).patience, null);
});

test("loot bag and overburdened", () => {
  assert.equal(battleOf(row({ ctxPct: 79 }), [], NOW).overburdened, false);
  assert.equal(battleOf(row({ ctxPct: 80 }), [], NOW).overburdened, true);
  const b = battleOf(row({ ctxPct: null }), [], NOW);
  assert.equal(b.bag, null); assert.equal(b.overburdened, false);
});

test("inn while compacting, then refreshed for 2 minutes", () => {
  const at = (secAgo) => battleOf(row({ compactedMs: NOW - secAgo * 1000 }), [], NOW);
  assert.deepEqual([at(30).inn, at(30).refreshed], [true, false]);
  assert.deepEqual([at(90).inn, at(90).refreshed], [false, true]);
  assert.deepEqual([at(200).inn, at(200).refreshed], [false, false]);
  assert.deepEqual([battleOf(row(), [], NOW).inn, battleOf(row(), [], NOW).refreshed], [false, false]);
});
