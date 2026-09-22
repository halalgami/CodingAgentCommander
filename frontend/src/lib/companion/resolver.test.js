import { test } from "node:test";
import assert from "node:assert/strict";
import { resolve, hydrateMemo, perTickProbability } from "./resolver.js";
import { EGG_TTL_MS, EGG_REFRACTORY_MS, TICKS_PER_MINUTE, CONDITION_CLASS } from "./pack.js";
import { MARATHON_MS, LONG_IDLE_MS } from "./conditions.js";

const NOW = 1_700_000_000_000;

function makeFacts(over = {}) {
  return {
    nowMs: NOW, hour: 14, weekday: 3,
    msSinceInput: 1000, msSinceOutput: 60_000,
    selectedActiveMs: 60_000, finishSeq: 0, recentFinishMs: [],
    firstRunOfDay: false, freshInstall: false,
    ...over,
  };
}

const packOf = (slots) => ({ schema: 1, canvas: { w: 832, h: 1216, scale: 2 }, slots });
const always = (v) => () => v;
// Deterministic LCG so a "rarity governs frequency" claim is measurable.
function lcg(seed) {
  let s = seed >>> 0;
  return () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296; };
}

test("rarity is per-minute and converts to per-tick, not used raw", () => {
  // A raw per-tick 0.02 fires within ~25 seconds at 2Hz, which is not rare by
  // any reading (spec §5.2).
  const p = perTickProbability(0.02);
  assert.ok(p < 0.0002 && p > 0.00016, `per-tick probability was ${p}`);
  // The round trip is the definition: not firing for a whole minute must have
  // probability 1 - r.
  assert.ok(Math.abs(Math.pow(1 - p, TICKS_PER_MINUTE) - 0.98) < 1e-12);
  assert.ok(Math.abs(Math.pow(1 - perTickProbability(0.5), TICKS_PER_MINUTE) - 0.5) < 1e-12);
  // ordering is preserved, and degenerate inputs are inert
  assert.ok(perTickProbability(0.5) > perTickProbability(0.02));
  assert.equal(perTickProbability(0), 0);
  assert.equal(perTickProbability(-1), 0);
  assert.equal(perTickProbability(undefined), 0);
  assert.equal(perTickProbability(1), 1);
  // the tick rate is an argument, so the authored number survives a rate change
  assert.ok(Math.abs(Math.pow(1 - perTickProbability(0.25, 60), 60) - 0.75) < 1e-12);
});

test("hydrateMemo drops runtime-only fields and prunes dead cooldowns", () => {
  const raw = {
    activeEgg: { id: "secret", slot: "idle", shownAt: NOW - 500_000 },
    eggCooldowns: { live: NOW + 10_000, dead: NOW - 10_000, junk: "nope" },
    stickyKey: "idle|neutral",
    pick: "day",
    prevFacts: { firstRunOfDay: true, finishSeq: 400 },
  };
  const m = hydrateMemo(raw, NOW);
  assert.equal(m.activeEgg, null, "an egg mid-display at quit must not resume");
  assert.equal(m.prevFacts, null, "a stale edge baseline suppresses events forever");
  assert.deepEqual(m.eggCooldowns, { live: NOW + 10_000 });
  assert.equal(m.stickyKey, "idle|neutral");
  assert.equal(m.pick, "day");
  assert.deepEqual(JSON.parse(JSON.stringify(m)), m, "hydrated memo must be JSON-clean");
  assert.deepEqual(hydrateMemo(null, NOW),
    { activeEgg: null, eggCooldowns: {}, stickyKey: null, pick: null, prevFacts: null });
});

test("a persisted prevFacts cannot suppress firstRunOfDay on a later day", () => {
  // The exact reason hydrateMemo exists.
  const pack = packOf({ idle: [{ id: "day" }, { id: "hello", when: "firstRunOfDay" }] });
  const memo = hydrateMemo({ prevFacts: { firstRunOfDay: true } }, NOW);
  const out = resolve({
    slot: "idle", pack, mood: "neutral",
    facts: makeFacts({ firstRunOfDay: true }), nowMs: NOW, rng: always(0.5), memo,
  });
  assert.equal(out.variant.id, "hello");
});

test("an egg is held for its full TTL, then released", () => {
  // The regression: the TTL was written but never read, the cooldown guard
  // failed on the next tick, and the egg showed for one tick — shorter than the
  // crossfade (spec §5.7 step 0).
  const pack = packOf({ idle: [{ id: "day" }, { id: "secret", rarity: 0.5 }] });
  const memo = hydrateMemo(null, NOW);
  const tick = (nowMs, rng) =>
    resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts({ nowMs }), nowMs, rng, memo });

  assert.equal(tick(NOW, always(0)).variant.id, "secret");
  assert.equal(memo.activeEgg.shownAt, NOW);
  // held even though nothing rolls on these ticks
  assert.equal(tick(NOW + 500, always(1)).variant.id, "secret");
  assert.equal(tick(NOW + EGG_TTL_MS - 1, always(1)).variant.id, "secret");
  // released exactly at the TTL
  assert.equal(tick(NOW + EGG_TTL_MS, always(1)).variant.id, "day");
  assert.equal(memo.activeEgg, null);
});

test("the per-egg refractory blocks a re-roll, then expires", () => {
  const pack = packOf({ idle: [{ id: "day" }, { id: "secret", rarity: 0.5 }] });
  const memo = hydrateMemo(null, NOW);
  const tick = (nowMs, rng) =>
    resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts({ nowMs }), nowMs, rng, memo });

  assert.equal(tick(NOW, always(0)).variant.id, "secret");
  assert.equal(memo.eggCooldowns.secret, NOW + EGG_TTL_MS + EGG_REFRACTORY_MS);
  const until = memo.eggCooldowns.secret;
  // a guaranteed-passing roll is refused for the whole refractory
  assert.equal(tick(until - 1, always(0)).variant.id, "day");
  assert.equal(tick(until, always(0)).variant.id, "secret");
});

test("an event outranks a rare roll on the same tick", () => {
  const pack = packOf({
    done: [{ id: "d" }, { id: "lucky", rarity: 0.5 }, { id: "hundred", when: "century" }],
  });
  const memo = hydrateMemo(null, NOW);
  const facts = (finishSeq, nowMs) => makeFacts({ finishSeq, nowMs });
  resolve({ slot: "done", pack, mood: "neutral", facts: facts(99, NOW), nowMs: NOW, rng: always(1), memo });
  const out = resolve({
    slot: "done", pack, mood: "neutral",
    facts: facts(100, NOW + 500), nowMs: NOW + 500, rng: always(0), memo,
  });
  assert.equal(out.variant.id, "hundred", "a roll beat an edge");
});

test("simultaneous eggs tie-break by weighted rng, never by array order", () => {
  const variants = [
    { id: "ev1", when: "century", weight: 1 },
    { id: "ev2", when: "streak", weight: 1 },
  ];
  const run = (rng, order) => {
    const pack = packOf({ done: [{ id: "d" }, ...order] });
    const memo = hydrateMemo(null, NOW);
    resolve({ slot: "done", pack, mood: "neutral",
      facts: makeFacts({ finishSeq: 99, recentFinishMs: [1, 2] }), nowMs: NOW, rng, memo });
    return resolve({ slot: "done", pack, mood: "neutral",
      facts: makeFacts({ finishSeq: 100, recentFinishMs: [1, 2, 3], nowMs: NOW + 500 }),
      nowMs: NOW + 500, rng, memo }).variant.id;
  };
  assert.equal(run(always(0), variants), "ev1");
  assert.equal(run(always(0.99), variants), "ev2");

  // Equal weights must split roughly 50/50 under a varying rng. The two
  // asserts above alone are not enough: they pin two single points, and a
  // reversed-array assertion at rng=0 (the original form of this check) is
  // satisfied by a broken "always return hits[0]" implementation just as
  // well as by a real weighted pick, because hits[0] happens to be "ev2"
  // once the array is reversed — same output, wrong reason. A distribution
  // sweep cannot be fooled that way: hits[0]-only collapses to 100%/0%.
  const rng = lcg(2026);
  let ev1 = 0, ev2 = 0;
  for (let i = 0; i < 400; i++) {
    const id = run(rng, variants);
    if (id === "ev1") ev1 += 1;
    else if (id === "ev2") ev2 += 1;
  }
  assert.equal(ev1 + ev2, 400, "the tie-break returned something other than ev1/ev2");
  assert.ok(Math.abs(ev1 - ev2) < 60, `tie-break was not ~50/50: ev1=${ev1} ev2=${ev2}`);
});

test("within a class the lowest rarity wins regardless of manifest order", () => {
  const rare = [{ id: "common", rarity: 0.5 }, { id: "rarest", rarity: 0.02 }];
  const run = (order) => {
    const pack = packOf({ idle: [{ id: "day" }, ...order] });
    const memo = hydrateMemo(null, NOW);
    return resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
      nowMs: NOW, rng: always(0), memo }).variant.id;
  };
  assert.equal(run(rare), "rarest");
  assert.equal(run([...rare].reverse()), "rarest");
});

test("the refractory does not flatten rarity into one frequency", () => {
  // The regression: a long cooldown dominated duty cycle, so 0.5 and 0.02
  // differed by 4% in on-screen time (spec §5.7). The assertion is on the
  // PROPERTY. If a seed produces a degenerate sample, change the seed constant
  // below — never the inequality.
  const fires = (rarity) => {
    const pack = packOf({ idle: [{ id: "day" }, { id: "egg", rarity }] });
    const memo = hydrateMemo(null, NOW);
    const rng = lcg(20260828);
    let count = 0, wasEgg = false;
    for (let i = 0; i < 2000; i++) {
      const nowMs = NOW + i * 500;
      resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts({ nowMs }), nowMs, rng, memo });
      const isEgg = memo.activeEgg !== null;
      if (isEgg && !wasEgg) count += 1;
      wasEgg = isEgg;
    }
    return count;
  };
  const common = fires(0.5);
  const rarest = fires(0.02);
  assert.ok(common >= 3, `a 0.5/min egg fired only ${common} times in ~17 min`);
  assert.ok(rarest <= 2, `a 0.02/min egg fired ${rarest} times in ~17 min`);
  assert.ok(common > rarest);
});

test("ambient conditions join the base pool and never flash", () => {
  // The regression: lateNight fired as a 12s egg every ten minutes all night
  // instead of being the night's art (spec §5.5).
  const pack = packOf({ idle: [{ id: "day" }, { id: "night", when: "lateNight" }] });
  const memo = hydrateMemo(null, NOW);
  let seen = new Set();
  for (let i = 0; i < 300; i++) {   // 150s at 2Hz
    const nowMs = NOW + i * 500;
    const out = resolve({ slot: "idle", pack, mood: "neutral",
      facts: makeFacts({ hour: 3, nowMs }), nowMs, rng: always(0.99), memo });
    seen.add(out.variant.id);
    assert.equal(memo.activeEgg, null, `ambient art became an egg at tick ${i}`);
    assert.equal(memo.eggCooldowns.night, undefined, "ambient art took a cooldown");
  }
  assert.deepEqual([...seen], ["night"], "ambient art did not hold steady");
});

test("an ambient condition that is false leaves its art out of the pool", () => {
  const pack = packOf({ idle: [{ id: "day" }, { id: "night", when: "lateNight" }] });
  const memo = hydrateMemo(null, NOW);
  const out = resolve({ slot: "idle", pack, mood: "neutral",
    facts: makeFacts({ hour: 14 }), nowMs: NOW, rng: always(0.99), memo });
  assert.equal(out.variant.id, "day");
});

test("an event fires once per rising edge, not once per tick it stays true", () => {
  const pack = packOf({ done: [{ id: "d" }, { id: "hundred", when: "century" }] });
  const memo = hydrateMemo(null, NOW);
  let fires = 0, wasEgg = false;
  const tick = (i, finishSeq) => {
    const nowMs = NOW + i * 500;
    resolve({ slot: "done", pack, mood: "neutral",
      facts: makeFacts({ finishSeq, nowMs }), nowMs, rng: always(1), memo });
    const isEgg = memo.activeEgg !== null;
    if (isEgg && !wasEgg) fires += 1;
    wasEgg = isEgg;
  };
  tick(0, 99);
  for (let i = 1; i < 300; i++) tick(i, 100);   // the counter sits on 100 for 150s
  assert.equal(fires, 1, `century fired ${fires} times while the counter sat on 100`);
});

test("prevFacts is a snapshot, so a mutated facts object still produces edges", () => {
  // The host reuses one facts object per tick. Aliasing it would make prev ===
  // facts and no event would ever fire.
  const pack = packOf({ done: [{ id: "d" }, { id: "hundred", when: "century" }] });
  const memo = hydrateMemo(null, NOW);
  const facts = makeFacts({ finishSeq: 99 });
  resolve({ slot: "done", pack, mood: "neutral", facts, nowMs: NOW, rng: always(1), memo });
  facts.finishSeq = 100;               // mutated in place, same object
  facts.nowMs = NOW + 500;
  const out = resolve({ slot: "done", pack, mood: "neutral", facts, nowMs: NOW + 500,
    rng: always(1), memo });
  assert.equal(out.variant.id, "hundred");
});

test("the sticky pick holds within a (slot, mood) and re-picks when mood changes", () => {
  const pack = packOf({ idle: [{ id: "a" }, { id: "b", mood: "happy" }] });
  const memo = hydrateMemo(null, NOW);
  const at = (mood, nowMs) => resolve({ slot: "idle", pack, mood,
    facts: makeFacts({ nowMs }), nowMs, rng: always(0.5), memo }).variant.id;

  // neutral: weights are [1, 0.25]; rng 0.5 lands on "a"
  assert.equal(at("neutral", NOW), "a");
  for (let i = 1; i < 40; i++) assert.equal(at("neutral", NOW + i * 500), "a", `re-picked at ${i}`);
  assert.equal(memo.stickyKey, "idle|neutral");

  // happy: weights become [1, 4]; the same rng now lands on the happy variant
  assert.equal(at("happy", NOW + 40_000), "b");
  assert.equal(memo.stickyKey, "idle|happy");
});

test("mood boosts weight, it never filters", () => {
  // A filter zeroes egg probability and collapses the pool: a slot holding only
  // mood-tagged art would render nothing at all.
  const onlyTagged = packOf({ idle: [{ id: "m", mood: "happy" }] });
  const memo = hydrateMemo(null, NOW);
  assert.equal(resolve({ slot: "idle", pack: onlyTagged, mood: "neutral",
    facts: makeFacts(), nowMs: NOW, rng: always(0.5), memo }).variant.id, "m");

  // an off-mood variant stays reachable alongside an untagged one
  const mixed = packOf({ idle: [{ id: "plain" }, { id: "m", mood: "happy" }] });
  const memo2 = hydrateMemo(null, NOW);
  assert.equal(resolve({ slot: "idle", pack: mixed, mood: "surprised",
    facts: makeFacts(), nowMs: NOW, rng: always(0.99), memo: memo2 }).variant.id, "m");
});

test("an empty base walks to idle and re-evaluates conditions there", () => {
  // Not "returns idle's first variant": steps 1-5 run again for idle, so an
  // ambient-gated idle variant is eligible.
  const pack = packOf({
    bored: [{ id: "never", rarity: 0.02 }],   // eggs only -> base is empty
    idle: [{ id: "day" }, { id: "night", when: "lateNight" }],
  });
  const memo = hydrateMemo(null, NOW);
  const out = resolve({ slot: "bored", pack, mood: "neutral",
    facts: makeFacts({ hour: 3 }), nowMs: NOW, rng: always(0.99), memo });
  assert.equal(out.variant.id, "night");
  // The sticky key must use the RESOLVED slot ("idle", not "bored") and now
  // also carries the ambient pool composition (C1) — "night" is eligible here
  // only because lateNight is true, so it appears in the key's suffix.
  assert.equal(memo.stickyKey, "idle|neutral|night");
});

test("a missing slot walks to idle, and an empty pack yields the placeholder", () => {
  const pack = packOf({ idle: [{ id: "day" }] });
  const memo = hydrateMemo(null, NOW);
  assert.equal(resolve({ slot: "working", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo }).variant.id, "day");

  const empty = packOf({});
  const memo2 = hydrateMemo(null, NOW);
  assert.equal(resolve({ slot: "working", pack: empty, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo: memo2 }).variant, null);
  assert.equal(resolve({ slot: "idle", pack: empty, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo: memo2 }).variant, null);
});

test("variants without a usable id are ignored rather than crashing", () => {
  const pack = packOf({ idle: [null, { file: "x.png" }, { id: "" }, { id: "ok" }] });
  const memo = hydrateMemo(null, NOW);
  assert.equal(resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo }).variant.id, "ok");
});

test("the memo round-trips through JSON unchanged after a long run", () => {
  const pack = packOf({
    idle: [{ id: "day" }, { id: "night", when: "lateNight" }, { id: "secret", rarity: 0.5 }],
    done: [{ id: "d" }, { id: "hundred", when: "century" }],
  });
  const memo = hydrateMemo(null, NOW);
  const rng = lcg(7);
  for (let i = 0; i < 600; i++) {
    const nowMs = NOW + i * 500;
    resolve({
      slot: i % 7 === 0 ? "done" : "idle", pack, mood: i % 3 === 0 ? "happy" : "neutral",
      facts: makeFacts({ nowMs, hour: i % 2 ? 3 : 14, finishSeq: 99 + Math.floor(i / 50) }),
      nowMs, rng, memo,
    });
  }
  // Force one egg so the cooldown map and activeEgg are both populated,
  // independent of what the seed happened to roll during the loop.
  const late = NOW + 600_000;
  resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts({ nowMs: late }),
    nowMs: late, rng: always(0), memo });

  const round = JSON.parse(JSON.stringify(memo));
  assert.deepEqual(round, memo, "memo carries a value JSON cannot represent");
  assert.equal(Object.getPrototypeOf(memo.eggCooldowns), Object.prototype);
  assert.ok(Object.keys(memo.eggCooldowns).length > 0);
  assert.notEqual(memo.activeEgg, null);
});

test("resolve returns the caller's memo object by identity", () => {
  const pack = packOf({ idle: [{ id: "day" }] });
  const memo = hydrateMemo(null, NOW);
  const out = resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo });
  assert.equal(out.memo, memo);
});

// --- C1: an ambient rising edge must be able to displace a live sticky pick ---

test("an ambient rising edge invalidates a live sticky pick, so entering art can display", () => {
  // Regression: ambient art JOINS the base pool without evicting the
  // incumbent (see the block comment at the top of the file). If pool
  // composition were not part of the sticky key, a pick made before the
  // ambient condition turned true would be re-served forever, and
  // marathon/longIdle/lateNight art could never reach the screen on a
  // machine left running (spec §5.5's whole point: "lateNight art IS the
  // night's art").
  const pack = packOf({ idle: [{ id: "day" }, { id: "night", when: "lateNight" }] });
  const memo = hydrateMemo(null, NOW);
  const at = (hour, nowMs) => resolve({ slot: "idle", pack, mood: "neutral",
    facts: makeFacts({ hour, nowMs }), nowMs, rng: always(0.99), memo }).variant.id;

  // Establish a sticky pick while the pool is just [day] (a single-item list
  // always resolves to that item regardless of rng — see weightedPick's clamp).
  assert.equal(at(14, NOW), "day");
  assert.equal(memo.stickyKey, "idle|neutral");
  for (let i = 1; i < 10; i++) assert.equal(at(14, NOW + i * 500), "day", `re-picked at ${i}`);

  // The clock crosses into lateNight with the SAME live memo. "night" joins
  // base; a stale key would keep serving "day" forever (the bug).
  assert.equal(at(3, NOW + 10_000), "night", "ambient art never reached the screen on entry");
  assert.equal(memo.stickyKey, "idle|neutral|night");
  for (let i = 1; i < 10; i++) {
    assert.equal(at(3, NOW + 10_000 + i * 500), "night", `not sticky after entry, tick ${i}`);
  }

  // The reverse (already worked pre-fix, asserted here for completeness): the
  // ambient condition goes false again and the plain pool returns.
  assert.equal(at(14, NOW + 30_000), "day");
  assert.equal(memo.stickyKey, "idle|neutral");
});

// --- I2: the sticky branch itself, and hydrateMemo's stale-pick guard ---

test("the sticky pick survives rng churn, not just a lucky reweight", () => {
  // Every other resolver test uses a constant rng, so a real sticky branch
  // and a non-sticky one that happens to reweight identically every tick are
  // indistinguishable. A varying rng tells them apart: only real stickiness
  // keeps the id constant across ticks that would otherwise reweight
  // differently.
  const pack = packOf({ idle: [{ id: "a" }, { id: "b" }, { id: "c" }] });
  const memo = hydrateMemo(null, NOW);
  const rng = lcg(42);
  const first = resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng, memo }).variant.id;
  for (let i = 1; i < 40; i++) {
    const nowMs = NOW + i * 500;
    const id = resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts({ nowMs }),
      nowMs, rng, memo }).variant.id;
    assert.equal(id, first, `pick drifted at tick ${i} despite unchanged slot+mood`);
  }
});

test("hydrateMemo's persisted pick cannot resurrect an id no longer in the pack", () => {
  const pack = packOf({ idle: [{ id: "a" }, { id: "b" }] });
  const memo = hydrateMemo({ stickyKey: "idle|neutral", pick: "GONE" }, NOW);
  const out = resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo });
  assert.ok(pack.slots.idle.some((v) => v.id === out.variant.id),
    "a stale persisted pick resurrected an id absent from the current pack");
});

// --- I3: mood weighting magnitudes ---

test("mood boost (x4) and demotion (x0.25) set the actual selection share, not just the winner", () => {
  // The existing "sticky pick" test only proves boost >= 1 (rng=0.5 selects
  // index 1 for both [1,4] and [1,1] weights, a floating-point coincidence).
  // Sweep rng with a FRESH memo per tick (stickiness would otherwise freeze
  // the very first pick and hide the distribution entirely).
  const N = 4000;
  const rng = lcg(99);
  const shareOf = (pack, mood, id) => {
    let hits = 0;
    for (let i = 0; i < N; i++) {
      const memo = hydrateMemo(null, NOW);
      if (resolve({ slot: "idle", pack, mood, facts: makeFacts(),
        nowMs: NOW, rng, memo }).variant.id === id) hits += 1;
    }
    return hits / N;
  };

  // boosted (x4) vs untagged (x1): expected share 4/5 = 0.8
  const untagged = packOf({ idle: [{ id: "on", mood: "happy" }, { id: "off" }] });
  const share1 = shareOf(untagged, "happy", "on");
  assert.ok(Math.abs(share1 - 0.8) < 0.03, `on-mood share was ${share1}, expected ~0.8`);

  // boosted (x4) vs off-mood demoted (x0.25): expected share 4/4.25 ≈ 0.941
  const offMood = packOf({ idle: [{ id: "on", mood: "happy" }, { id: "off", mood: "sad" }] });
  const share2 = shareOf(offMood, "happy", "on");
  assert.ok(Math.abs(share2 - 4 / 4.25) < 0.02, `on-mood share was ${share2}, expected ~0.941`);
});

// --- I4: `when` + `rarity` combined, both classes ---

test("an event's `when` gates AND its `rarity` still rolls (spec §5.2)", () => {
  const pack = packOf({ done: [{ id: "d" }, { id: "hundred", when: "century", rarity: 0.02 }] });

  // the edge is true (99 -> 100) but the roll must still run, and can fail
  const memoFail = hydrateMemo(null, NOW);
  resolve({ slot: "done", pack, mood: "neutral", facts: makeFacts({ finishSeq: 99 }),
    nowMs: NOW, rng: always(0.5), memo: memoFail });
  const failed = resolve({ slot: "done", pack, mood: "neutral",
    facts: makeFacts({ finishSeq: 100, nowMs: NOW + 500 }), nowMs: NOW + 500,
    rng: always(0.5), memo: memoFail });
  assert.equal(failed.variant.id, "d", "the roll must be able to suppress a gated event");

  // the same edge, this time with a roll certain to pass
  const memoPass = hydrateMemo(null, NOW);
  resolve({ slot: "done", pack, mood: "neutral", facts: makeFacts({ finishSeq: 99 }),
    nowMs: NOW, rng: always(0), memo: memoPass });
  const passed = resolve({ slot: "done", pack, mood: "neutral",
    facts: makeFacts({ finishSeq: 100, nowMs: NOW + 500 }), nowMs: NOW + 500,
    rng: always(0), memo: memoPass });
  assert.equal(passed.variant.id, "hundred");
});

test("an ambient `when` gates AND its `rarity` still rolls, making it a rare egg not always-on ambient", () => {
  const pack = packOf({ idle: [{ id: "day" }, { id: "night", when: "lateNight", rarity: 0.02 }] });

  // the condition is true (hour 3) but the roll fails every tick: a
  // rarity-gated ambient variant must never leak into the ordinary base pool
  const memoFail = hydrateMemo(null, NOW);
  const seen = new Set();
  for (let i = 0; i < 50; i++) {
    const nowMs = NOW + i * 500;
    const out = resolve({ slot: "idle", pack, mood: "neutral",
      facts: makeFacts({ hour: 3, nowMs }), nowMs, rng: always(0.99), memo: memoFail });
    seen.add(out.variant.id);
    assert.equal(memoFail.activeEgg, null, `tick ${i}: a failed roll must not become an egg`);
  }
  assert.deepEqual([...seen], ["day"], "a rarity-gated ambient variant leaked into the base pool");

  // the same condition, this time with a roll certain to pass: it must show
  // up as an EGG (TTL + cooldown), never as ordinary sticky ambient art
  const memoPass = hydrateMemo(null, NOW);
  const out = resolve({ slot: "idle", pack, mood: "neutral",
    facts: makeFacts({ hour: 3 }), nowMs: NOW, rng: always(0), memo: memoPass });
  assert.equal(out.variant.id, "night");
  assert.notEqual(memoPass.activeEgg, null, "a gated-rare hit must become an egg");
  assert.equal(memoPass.eggCooldowns.night, NOW + EGG_TTL_MS + EGG_REFRACTORY_MS);
});

// --- I5: `weight` ---

test("`weight` sets relative selection frequency (documented manifest field)", () => {
  const N = 4000;
  const rng = lcg(123);
  const pack = packOf({ idle: [{ id: "heavy", weight: 3 }, { id: "light", weight: 1 }] });
  let heavy = 0;
  for (let i = 0; i < N; i++) {
    const memo = hydrateMemo(null, NOW); // fresh memo: defeat stickiness
    if (resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
      nowMs: NOW, rng, memo }).variant.id === "heavy") heavy += 1;
  }
  const share = heavy / N;
  assert.ok(Math.abs(share - 0.75) < 0.03, `heavy share was ${share}, expected ~0.75 (weight 3 vs 1)`);
});

// --- I6: snapshotFacts must carry every field CONDITION_CLASS's predicates read ---

const DRIVE = {
  lateNight: { off: { hour: 14 }, on: { hour: 3 } },
  weekend: { off: { weekday: 3 }, on: { weekday: 6 } },
  marathon: { off: { selectedActiveMs: 0 }, on: { selectedActiveMs: MARATHON_MS + 1 } },
  longIdle: { off: { msSinceInput: 0 }, on: { msSinceInput: LONG_IDLE_MS + 1 } },
  freshInstall: { off: { freshInstall: false }, on: { freshInstall: true } },
  firstRunOfDay: { off: { firstRunOfDay: false }, on: { firstRunOfDay: true } },
  century: { off: { finishSeq: 99 }, on: { finishSeq: 100 } },
  streak: { off: { recentFinishMs: [] }, on: { recentFinishMs: [1, 2, 3] } },
};
const SLOT_FOR = {
  lateNight: "idle", weekend: "idle", freshInstall: "idle", firstRunOfDay: "idle",
  marathon: "working", longIdle: "working", century: "done", streak: "done",
};

test("snapshotFacts carries every field CONDITION_CLASS's predicates read, for every condition", () => {
  // snapshotFacts is a hand-maintained allowlist with nothing statically
  // tying it to conditions.js. Drive a rising edge for EVERY named condition
  // through resolve() and require the predicate to see it exactly once. For
  // event-class conditions this is the real teeth: drop `recentFinishMs` or
  // `firstRunOfDay` (the two examples this finding names) and the baseline
  // reads permanently false, so the condition re-fires the instant the
  // refractory clears instead of staying fired. Ambient-class conditions
  // never consult `prev` (see evaluate() in conditions.js), so their entries
  // here are a "stays visible" sanity check rather than a snapshot-drop
  // trap — the table is still exhaustive so a FUTURE event-class condition
  // is automatically covered.
  for (const name of Object.keys(CONDITION_CLASS)) {
    const slot = SLOT_FOR[name];
    const { off, on } = DRIVE[name];
    const pack = packOf({ [slot]: [{ id: "base" }, { id: "target", when: name }] });
    const memo = hydrateMemo(null, NOW);
    // rng=0.99: for the event-class names "target" is the only hit regardless
    // of rng, but for ambient-class names "target" only WINS the reweight
    // (vs. the untagged "base") when rng favors the later array slot — the
    // point of the assertion is that it is eligible, i.e. present in `base`.
    const tick = (factsOver, nowMs) => resolve({ slot, pack, mood: "neutral",
      facts: makeFacts({ ...factsOver, nowMs }), nowMs, rng: always(0.99), memo });

    tick(off, NOW);                     // baseline: condition false
    const first = tick(on, NOW + 500);  // rising edge
    assert.equal(first.variant.id, "target", `${name}: rising edge was not detected`);

    if (CONDITION_CLASS[name] === "event") {
      // held true for a long stretch: must fire exactly once, not re-fire
      // once the refractory clears. wasEgg starts true: `first` (above) is
      // already the one legitimate fire, so the counting loop must not
      // recount the egg it opened.
      let fires = 0, wasEgg = true;
      for (let i = 1; i < 300; i++) {
        const nowMs = NOW + 500 + i * 500;
        tick(on, nowMs);
        const isEgg = memo.activeEgg !== null;
        if (isEgg && !wasEgg) fires += 1;
        wasEgg = isEgg;
      }
      assert.equal(fires, 0, `${name}: re-fired while held true (baseline never advanced)`);
    } else {
      // ambient: stays visible for as long as the condition holds.
      for (let i = 1; i < 10; i++) {
        const nowMs = NOW + 500 + i * 500;
        assert.equal(tick(on, nowMs).variant.id, "target", `${name}: ambient art dropped out while true`);
      }
    }
  }
});

// --- I8: the edge baseline must advance on every held tick, not just the tick that opened the egg ---

test("the edge baseline advances on every held tick, so a sibling variant cannot replay it 12s late", () => {
  // Removing the prevFacts write from the hold path is invisible with a
  // single event variant: its own refractory blocks a same-id re-fire
  // regardless of the baseline. Use TWO variants on the SAME condition, and
  // change finishSeq AGAIN while the first is being held: only a baseline
  // that keeps advancing during the hold can tell the second variant "no,
  // that crossing was already consumed" once the hold releases.
  const pack = packOf({
    done: [{ id: "d" }, { id: "hundred_a", when: "century" }, { id: "hundred_b", when: "century" }],
  });
  const memo = hydrateMemo(null, NOW);
  const tick = (finishSeq, nowMs) =>
    resolve({ slot: "done", pack, mood: "neutral", facts: makeFacts({ finishSeq, nowMs }),
      nowMs, rng: always(0), memo });

  tick(99, NOW);                                    // baseline
  const T1 = NOW + 500;
  assert.equal(tick(100, T1).variant.id, "hundred_a", "rng=0 ties to the first manifest entry");

  // a SECOND crossing happens while "hundred_a" is still being held (TTL is
  // 24 ticks at 500ms; stay strictly inside it)
  for (let i = 1; i < 24; i++) {
    assert.equal(tick(250, T1 + i * 500).variant.id, "hundred_a", `held tick ${i}`);
  }

  // released: a current baseline sees finishSeq already at 250, so neither
  // "hundred_a" (its own refractory) nor "hundred_b" (no NEW edge) fires.
  const released = tick(250, T1 + 24 * 500);
  assert.equal(released.variant.id, "d", "a stale hold-path baseline let a sibling replay the edge");
});

// --- M9: resolve must accept a frozen memo ---

test("resolve accepts a frozen memo instead of throwing", () => {
  // The memo is caller-owned and may live in frozen Svelte state; resolve
  // must copy rather than mutate in place when it cannot write.
  const pack = packOf({ idle: [{ id: "day" }] });
  const frozen = Object.freeze({
    activeEgg: null, eggCooldowns: {}, stickyKey: null, pick: null, prevFacts: null,
  });
  const out = resolve({ slot: "idle", pack, mood: "neutral", facts: makeFacts(),
    nowMs: NOW, rng: always(0.5), memo: frozen });
  assert.equal(out.variant.id, "day");
  assert.notEqual(out.memo, frozen, "resolve must not claim the frozen input as the live memo");
  assert.equal(out.memo.stickyKey, "idle|neutral");
});

// --- M10: hydrateMemo cooldown clamping and default nowMs ---

test("hydrateMemo clamps a cooldown timestamp corrupted by forward clock skew", () => {
  // If the sidecar was written while the host clock was fast, a persisted
  // cooldown could sit years in the future and disable that egg permanently.
  const raw = { eggCooldowns: { skewed: NOW + 1000 * 60 * 60 * 24 * 365 } }; // a year out
  const m = hydrateMemo(raw, NOW);
  assert.equal(m.eggCooldowns.skewed, NOW + EGG_TTL_MS + EGG_REFRACTORY_MS);
});

test("hydrateMemo defaults nowMs instead of silently dropping every cooldown", () => {
  const raw = { eggCooldowns: { live: Date.now() + 60_000 } };
  const m = hydrateMemo(raw); // nowMs omitted
  assert.deepEqual(m.eggCooldowns, { live: raw.eggCooldowns.live });
});
