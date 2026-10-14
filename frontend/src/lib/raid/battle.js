// frontend/src/lib/raid/battle.js
// Battle state (spec §4): what a session's fight looks like right now. HP
// counts tool marks in the current run. The River has no prompt mark, so a
// new run starts at a gap of more than 2 minutes after the assistant's answer
// (a think mark). A gap after a tool mark is that tool still running (a long
// test suite), and a permission pause is bracketed by wait marks. Pure.
import { compacting } from "../city/sites.js";

const GAP_MS = 2 * 60000;
const ENRAGE_MS = 10 * 60000;
const isHit = (e) => e.kind !== "think" && e.kind !== "wait";

function run(events = []) {
  const lead = events.filter((e) => !e.agent).sort((a, b) => a.at - b.at);
  const out = [];
  for (let i = lead.length - 1; i >= 0; i--) {
    const e = lead[i], newer = lead[i + 1];
    if (newer && newer.at - e.at > GAP_MS && e.kind === "think") break;
    out.push(e);
  }
  return out; // newest first
}

export const hitsOf = (events) => run(events).filter(isHit).length;

export function lastHitAt(events) {
  const e = run(events).find(isHit);
  return e ? e.at : 0;
}

const POSE = { charge: "stance", attack: "attack", orders: "await", clear: "victory", ko: "hurt", rest: "stance" };
const BOSS = { charge: "present", attack: "present", orders: "present", clear: "dissolve", ko: "present", rest: "absent" };

export function battleOf(row, events, now) {
  const a = row.activity;
  const working = a === "thinking" || a === "tool" || row.crewOut > 0;
  const state = a === "permission" ? "orders" : a === "error" ? "ko"
    : working ? (a === "tool" ? "attack" : "charge")
    : a === "finished" && row.unread ? "clear" : "rest";
  const age = row.sinceMs ? now - row.sinceMs : 0;
  const inn = compacting(row, now) != null;
  const sinceComp = row.compactedMs ? now - row.compactedMs : -1;
  return {
    state, pose: POSE[state], boss: BOSS[state],
    hp: Math.max(5, 100 - 7 * hitsOf(events)),
    enraged: working && age > ENRAGE_MS,
    patience: state !== "orders" ? null : age < 2 * 60000 ? "tapping" : age < ENRAGE_MS ? "sitting" : "asleep",
    bag: row.ctxPct ?? null,
    overburdened: (row.ctxPct ?? 0) >= 80,
    inn,
    refreshed: !inn && sinceComp > 60000 && sinceComp <= 3 * 60000,
  };
}
