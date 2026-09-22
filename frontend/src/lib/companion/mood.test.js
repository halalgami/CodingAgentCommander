import { test } from "node:test";
import assert from "node:assert/strict";
import { createMood, reactionInput, MOODS, STEADY_MOODS } from "./mood.js";
import { MOOD_DWELL_MS, MOOD_MAX_HOLD_MS, MOOD_REFRACTORY_MS } from "./pack.js";

const NOW = 1_700_000_000_000;

function makeState(sel = {}, over = {}) {
  return {
    sessions: [
      { windowID: "w1", name: "alpha", status: "active", statusSinceMs: NOW - 60_000,
        lastFinishMs: 0, errorMs: 0, ...sel },
    ],
    selected: "w1",
    running: 0, finished: 0, finishSeq: 0, lastFinished: "",
    ...over,
  };
}

function makeFacts(over = {}) {
  return {
    nowMs: NOW, hour: 14, weekday: 3,
    msSinceInput: 1000, msSinceOutput: 60_000,
    selectedActiveMs: 60_000, finishSeq: 0, recentFinishMs: [],
    firstRunOfDay: false, freshInstall: false,
    ...over,
  };
}

test("reactionInput supplies every field reactions.js reads", () => {
  // reactions.js reads s.quotaHigh, s.running, s.error, s.click, s.rc,
  // s.finished, s.awaiting, s.greeted, s.idleMs and s.nowMs. Any unsupplied
  // field would be undefined, every comparison false, and the whole system
  // would silently collapse to one constant expression (spec §5.4).
  const input = reactionInput(makeState(), makeFacts());
  assert.deepEqual(Object.keys(input).sort(), [
    "awaiting", "click", "error", "finished", "greeted",
    "idleMs", "nowMs", "quotaHigh", "rc", "running",
  ]);
  for (const [k, v] of Object.entries(input)) {
    assert.notEqual(v, undefined, `${k} is undefined`);
  }
});

test("reactionInput hardcodes quotaHigh false while quota is deferred", () => {
  assert.equal(reactionInput(makeState(), makeFacts()).quotaHigh, false);
  assert.equal(reactionInput(makeState({}, { running: 5 }), makeFacts()).quotaHigh, false);
});

test("reactionInput derives error and awaiting from the selected session", () => {
  const errored = reactionInput(makeState({ errorMs: NOW - 1000 }), makeFacts());
  assert.equal(errored.error, true);
  const stale = reactionInput(makeState({ errorMs: NOW - 60_000 }), makeFacts());
  assert.equal(stale.error, false, "an error past ERROR_TTL_MS must not stick");

  const awaiting = reactionInput(makeState({ lastFinishMs: NOW - 10_000 }),
    makeFacts({ msSinceOutput: 30_000 }));
  assert.equal(awaiting.awaiting, true);
  const consumed = reactionInput(makeState({ lastFinishMs: NOW - 30_000 }),
    makeFacts({ msSinceOutput: 10_000 }));
  assert.equal(consumed.awaiting, false);
});

test("a quiet state settles on neutral and stays there", () => {
  const m = createMood({ now: () => NOW });
  const state = makeState();
  let seen = new Set();
  for (let i = 0; i < 200; i++) seen.add(m.update(state, makeFacts({ nowMs: NOW + i * 500 })));
  assert.deepEqual([...seen], ["neutral"], "a quiet state oscillated");
});

test("running sessions read as relaxed", () => {
  const m = createMood({ now: () => NOW });
  m.update(makeState(), makeFacts({ nowMs: NOW }));                 // prime
  const out = m.update(makeState({}, { running: 2 }), makeFacts({ nowMs: NOW + 500 }));
  assert.equal(out, "relaxed");
});

test("a finish reads as happy", () => {
  const m = createMood({ now: () => NOW });
  m.update(makeState(), makeFacts({ nowMs: NOW }));                 // prime at finished=0
  const out = m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: NOW + 500 }));
  assert.equal(out, "happy");
});

test("an error edge reads as surprised", () => {
  const m = createMood({ now: () => NOW });
  m.update(makeState(), makeFacts({ nowMs: NOW }));                 // prime
  const out = m.update(makeState({ errorMs: NOW + 400 }), makeFacts({ nowMs: NOW + 500 }));
  assert.equal(out, "surprised");
});

test("the first update primes: a mount into finished sessions does not flash", () => {
  // reactions.js treats a null prev as "finished was 0" and "error was false",
  // so mounting into an app with seven completed sessions would fire happy.
  const m = createMood({ now: () => NOW });
  const busy = makeState({ errorMs: NOW - 500 }, { finished: 7, running: 2 });
  const first = m.update(busy, makeFacts({ nowMs: NOW }));
  assert.ok(STEADY_MOODS.includes(first), `primed into transient ${first}`);
  const second = m.update(busy, makeFacts({ nowMs: NOW + 500 }));
  assert.equal(second, "relaxed", "running sessions should settle to relaxed");
});

test("every value returned is a member of MOODS", () => {
  const m = createMood({ now: () => NOW });
  const outs = [
    m.update(makeState(), makeFacts({ nowMs: NOW })),
    m.update(makeState({}, { running: 3 }), makeFacts({ nowMs: NOW + 500 })),
    m.update(makeState({}, { running: 3, finished: 1 }), makeFacts({ nowMs: NOW + 1000 })),
    m.update(makeState({ errorMs: NOW + 5000 }), makeFacts({ nowMs: NOW + 30_000 })),
  ];
  for (const o of outs) assert.ok(MOODS.includes(o), `${o} is not a mood`);
});

test("minimum dwell: a second transient inside MOOD_DWELL_MS does not displace", () => {
  const m = createMood({ now: () => NOW });
  m.update(makeState(), makeFacts({ nowMs: NOW }));                              // prime
  assert.equal(m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: NOW + 500 })), "happy");
  // an error edge 500ms later is inside the dwell window
  assert.equal(
    m.update(makeState({ errorMs: NOW + 900 }, { finished: 1 }), makeFacts({ nowMs: NOW + 1000 })),
    "happy", "a transient was cut short inside its dwell");
  // let the error go false so a fresh rising edge is possible
  m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: NOW + 1500 }));
  // past the dwell, a different transient does displace
  assert.equal(
    m.update(makeState({ errorMs: NOW + 3900 }, { finished: 1 }),
      makeFacts({ nowMs: NOW + 500 + MOOD_DWELL_MS + 500 })),
    "surprised");
});

test("maximum hold: a transient cannot outlast MOOD_MAX_HOLD_MS", () => {
  const m = createMood({ now: () => NOW });
  const quiet = makeState();
  m.update(quiet, makeFacts({ nowMs: NOW }));                                    // prime
  m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: NOW + 500 }));     // happy at +500
  const finished = makeState({}, { finished: 1 });
  let last = "happy";
  for (let t = NOW + 1000; t <= NOW + 500 + MOOD_MAX_HOLD_MS; t += 500) {
    last = m.update(finished, makeFacts({ nowMs: t }));
    if (t < NOW + 500 + MOOD_MAX_HOLD_MS) assert.equal(last, "happy", `dropped early at ${t - NOW}`);
  }
  assert.equal(last, "neutral", "happy outlasted MOOD_MAX_HOLD_MS");
});

test("post-expiry refractory stops the expired transient re-latching", () => {
  const m = createMood({ now: () => NOW });
  m.update(makeState(), makeFacts({ nowMs: NOW }));                              // prime
  m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: NOW + 500 }));     // happy
  const expiry = NOW + 500 + MOOD_MAX_HOLD_MS;
  assert.equal(m.update(makeState({}, { finished: 1 }), makeFacts({ nowMs: expiry })), "neutral");
  // a new finish immediately after expiry must NOT re-latch happy
  assert.equal(
    m.update(makeState({}, { finished: 2 }), makeFacts({ nowMs: expiry + 500 })),
    "neutral", "happy re-latched inside its refractory");
  // once the refractory is over it latches again
  assert.equal(
    m.update(makeState({}, { finished: 3 }), makeFacts({ nowMs: expiry + MOOD_REFRACTORY_MS })),
    "happy");
});

test("only neutral and relaxed are steady states over a long run", () => {
  const m = createMood({ now: () => NOW });
  let run = 0, worst = 0, lastMood = null, final = null;
  // 401 ticks, not 400: the last finish lands exactly at tick 360, and
  // MOOD_MAX_HOLD_MS (20000ms = 40 ticks) needs a full 40 ticks after that to
  // expire. A 400-tick loop stops one tick short of the expiry and the run
  // ends mid-hold, which would make this assertion about settling on a
  // steady state flaky by construction rather than a property of the latch.
  for (let i = 0; i < 401; i++) {
    const t = NOW + i * 500;
    // a finish every 120 ticks (60s), an error every 200 ticks
    const finished = Math.floor(i / 120);
    const errorMs = i % 200 === 50 ? t : 0;
    final = m.update(makeState({ errorMs }, { finished }), makeFacts({ nowMs: t }));
    if (final === lastMood) run += 1; else { run = 1; lastMood = final; }
    if (!STEADY_MOODS.includes(final)) worst = Math.max(worst, run);
  }
  // MOOD_MAX_HOLD_MS / 500ms ticks, plus the tick that latched it
  assert.ok(worst <= MOOD_MAX_HOLD_MS / 500 + 1, `a transient held for ${worst} ticks`);
  assert.ok(STEADY_MOODS.includes(final), `settled on transient ${final}`);
});

test("sad is unreachable in v1", () => {
  // Quota is deferred (spec §6.4): reactionInput hardcodes quotaHigh false and
  // `sad` is the only expression reactions.js derives from it.
  //
  // WHEN QUOTA LANDS, THIS TEST SHOULD FAIL. That is the point. Replace it with
  // a positive reachability test and wire quotaHigh to the cached poller
  // described in spec §6.4; do not weaken this assertion to keep it green.
  const m = createMood({ now: () => NOW });
  for (let i = 0; i < 300; i++) {
    const t = NOW + i * 500;
    const state = makeState(
      { errorMs: i % 7 === 0 ? t : 0, lastFinishMs: i % 5 === 0 ? t - 1000 : 0 },
      { running: i % 3, finished: Math.floor(i / 4) });
    const facts = makeFacts({
      nowMs: t,
      msSinceInput: i % 11 === 0 ? 600_000 : 1000,
      msSinceOutput: i % 2 === 0 ? 0 : 60_000,
    });
    assert.notEqual(m.update(state, facts), "sad", `sad became reachable at tick ${i}`);
  }
});
