// Picks one variant per tick from a slot, honouring ambient conditions, event
// eggs, rare eggs, mood and a sticky pick. Pure: time and randomness are
// arguments, the pack is data, and the only mutable thing is the caller's memo.
//
// Rules that exist because the alternative was a real bug:
//   - Step 0 READS the egg TTL. Writing it and not reading it showed eggs for a
//     single tick, shorter than the crossfade.
//   - Ambient conditions never flash: they join the base pool, weighted
//     normally, no TTL, no cooldown. lateNight art IS the night's art.
//   - Mood BOOSTS weight (x4) and demotes off-mood art (x0.25). It never
//     filters: a filter zeroes egg probability and collapses the pool.
//   - Nothing here uses `arrayA || arrayB`. An empty array is truthy, so that
//     idiom silently disables the empty-base fallback.
//   - The sticky key is (resolvedSlot, mood, ambient pool). Keyed on slot
//     alone, mood is inert: `base` does not change when mood changes, so the
//     weights would be consumed once per slot entry and never again. Keyed
//     without the ambient pool, an ambient condition that turns true while
//     slot and mood stay fixed can never be picked: it JOINS `base` without
//     evicting the incumbent (previous rule), so the sticky check keeps
//     returning the pre-existing pick forever — marathon, longIdle and
//     lateNight art would be permanently dead. The pool's composition, not
//     the raw facts, belongs in the key: only a CHANGE in what is eligible
//     should force a re-roll.
//   - `rarity` is per MINUTE the slot is showing, converted to per-tick here, so
//     the authored number survives a change to RESOLVE_HZ.
import { EGG_TTL_MS, EGG_REFRACTORY_MS, TICKS_PER_MINUTE } from "./pack.js";
import { evaluate, isEdge } from "./conditions.js";

export function perTickProbability(rarityPerMinute, ticksPerMinute = TICKS_PER_MINUTE) {
  const r = Number(rarityPerMinute);
  if (!(r > 0)) return 0;
  if (r >= 1) return 1;
  return 1 - Math.pow(1 - r, 1 / ticksPerMinute);
}

// Call once on the JSON parsed from the pack's sidecar. Keeps the egg
// refractories and the sticky pick (the whole point of persisting), drops the
// two runtime-only fields.
export function hydrateMemo(raw, nowMs = Date.now()) {
  const src = raw && typeof raw === "object" ? raw : {};
  const persisted = src.eggCooldowns && typeof src.eggCooldowns === "object" ? src.eggCooldowns : {};
  // A cooldown can never legitimately outlive one TTL+refractory from now:
  // that is the longest any real fire could have set it to. Clamping catches
  // a sidecar written under a forward-skewed host clock, which would
  // otherwise disable that egg forever instead of for one refractory window.
  const maxUntil = nowMs + EGG_TTL_MS + EGG_REFRACTORY_MS;
  const eggCooldowns = {};
  for (const [id, until] of Object.entries(persisted)) {
    if (typeof until === "number" && Number.isFinite(until) && until > nowMs) {
      eggCooldowns[id] = Math.min(until, maxUntil);
    }
  }
  return {
    activeEgg: null,
    eggCooldowns,
    stickyKey: typeof src.stickyKey === "string" ? src.stickyKey : null,
    pick: typeof src.pick === "string" ? src.pick : null,
    prevFacts: null,
  };
}

export function resolve({ slot, pack, mood, facts, nowMs, rng = Math.random, memo }) {
  const m = normalizeMemo(memo);
  const prev = m.prevFacts;

  // 0 — an egg owns the screen for its whole TTL.
  const egg = m.activeEgg;
  if (egg && nowMs - egg.shownAt < EGG_TTL_MS) {
    const held = findVariant(pack, egg.slot, egg.id);
    if (held) {
      m.prevFacts = snapshotFacts(facts);
      return { variant: held, memo: m };
    }
  }
  m.activeEgg = null; // expired, or the pack changed underneath it

  const ctx = { pack, mood, facts, prev, nowMs, rng, m };
  let variant = resolveSlot(slot, ctx);
  // 6 — an empty base walks to `idle`, re-evaluating steps 1-5 there.
  if (variant === null && slot !== "idle") variant = resolveSlot("idle", ctx);

  // Advance the edge baseline on EVERY tick, including egg-hold ticks, so an
  // edge is never replayed twelve seconds late.
  m.prevFacts = snapshotFacts(facts);
  return { variant, memo: m };
}

function resolveSlot(slotName, ctx) {
  const { pack, mood, facts, prev, nowMs, rng, m } = ctx;
  const list = variantsOf(pack, slotName);
  if (list.length === 0) return null;

  const ambient = [];
  const events = [];
  const rare = [];
  const plain = [];

  for (const v of list) {
    const rarity = typeof v.rarity === "number" && v.rarity > 0 ? v.rarity : 0;

    if (v.when && isEdge(v.when)) {                       // 2 — event eggs
      if (refractory(m, v.id, nowMs)) continue;
      if (!evaluate(v.when, facts, prev)) continue;
      // `when` gates and `rarity`, if present, still rolls (spec §5.2).
      if (rarity > 0 && !(rng() < perTickProbability(rarity))) continue;
      events.push(v);
      continue;
    }

    if (v.when) {                                         // 1 — ambient
      if (!evaluate(v.when, facts, prev)) continue;
      if (rarity > 0) {                                   // gated rare egg
        if (!refractory(m, v.id, nowMs) && rng() < perTickProbability(rarity)) rare.push(v);
        continue;
      }
      ambient.push(v);
      continue;
    }

    if (rarity > 0) {                                     // 3 — rare eggs
      if (!refractory(m, v.id, nowMs) && rng() < perTickProbability(rarity)) rare.push(v);
      continue;
    }

    plain.push(v);
  }

  // 4 — an edge happened; a roll merely succeeded.
  const hits = events.length > 0 ? events : rare;
  if (hits.length > 0) {
    const chosen = pickEgg(hits, rng);
    m.activeEgg = { id: chosen.id, slot: slotName, shownAt: nowMs };
    m.eggCooldowns[chosen.id] = nowMs + EGG_TTL_MS + EGG_REFRACTORY_MS;
    return chosen;
  }

  // 5 — base pool.
  const base = plain.concat(ambient);
  if (base.length === 0) return null;

  // 7 — sticky on (resolved slot, mood, ambient pool composition). Ambient art
  // JOINS `base` without evicting the incumbent pick (rule above), so if the
  // key omitted which ambient conditions are currently true, a pick made
  // before an ambient condition turned true would keep being served forever —
  // the entering art would sit in `base`, eligible, but never re-rolled into
  // (C1). Keying on the sorted ambient ids forces a re-roll exactly when pool
  // membership changes, and only then. Suffix is omitted when ambient is
  // empty so the common (no ambient art) key stays exactly `slot|mood`.
  const ambientKey = ambient.map((v) => v.id).sort().join(",");
  const key = `${slotName}|${mood ?? ""}${ambientKey ? `|${ambientKey}` : ""}`;
  if (m.stickyKey === key && m.pick) {
    const kept = base.find((v) => v.id === m.pick);
    if (kept) return kept;
  }

  // 8 — weighted pick.
  const picked = weightedPick(base, mood, rng);
  m.stickyKey = key;
  m.pick = picked.id;
  return picked;
}

function pickEgg(hits, rng) {
  const withRarity = hits.filter((v) => typeof v.rarity === "number" && v.rarity > 0);
  if (withRarity.length > 0) {
    let min = Infinity;
    for (const v of withRarity) if (v.rarity < min) min = v.rarity;
    const tied = withRarity.filter((v) => v.rarity === min);
    if (tied.length === 1) return tied[0];
    return weightedPick(tied, null, rng);
  }
  return weightedPick(hits, null, rng);
}

function moodFactor(v, mood) {
  if (mood == null) return 1;
  if (v.mood === mood) return 4;   // boost
  if (v.mood) return 0.25;         // demote, never zero
  return 1;
}

function weightedPick(list, mood, rng) {
  const weights = list.map((v) => {
    const w = typeof v.weight === "number" && v.weight >= 0 ? v.weight : 1;
    return w * moodFactor(v, mood);
  });
  const total = weights.reduce((a, b) => a + b, 0);
  if (!(total > 0)) {
    const i = Math.floor(rng() * list.length);
    return list[Math.min(Math.max(i, 0), list.length - 1)];
  }
  let r = rng() * total;
  for (let i = 0; i < list.length; i++) {
    r -= weights[i];
    if (r < 0) return list[i];
  }
  return list[list.length - 1];
}

function variantsOf(pack, slotName) {
  const slots = pack && typeof pack === "object" ? pack.slots : null;
  const list = slots && Array.isArray(slots[slotName]) ? slots[slotName] : [];
  return list.filter((v) => v && typeof v.id === "string" && v.id !== "");
}

function findVariant(pack, slotName, id) {
  return variantsOf(pack, slotName).find((v) => v.id === id) ?? null;
}

function refractory(m, id, nowMs) {
  const until = m.eggCooldowns[id];
  return typeof until === "number" && until > nowMs;
}

function normalizeMemo(memo) {
  const src = memo && typeof memo === "object" ? memo : {};
  // The memo is caller-owned and will live in Svelte state, which may freeze
  // snapshots. Copy rather than mutate in place when we cannot write — the
  // alternative is `resolve` throwing "Cannot add property stickyKey, object
  // is not extensible" on every frozen memo (M9).
  const m = Object.isExtensible(src) ? src : { ...src };
  if (!m.eggCooldowns || typeof m.eggCooldowns !== "object") m.eggCooldowns = {};
  if (typeof m.stickyKey !== "string") m.stickyKey = null;
  if (typeof m.pick !== "string") m.pick = null;
  if (!m.activeEgg || typeof m.activeEgg !== "object") m.activeEgg = null;
  if (!m.prevFacts || typeof m.prevFacts !== "object") m.prevFacts = null;
  return m;
}

// A COPY, never a reference: the host reuses one facts object, and aliasing it
// would make prev identical to facts and no edge would ever fire. Every field
// is coerced to a JSON-clean value because the memo is persisted.
function snapshotFacts(facts) {
  if (!facts) return null;
  const num = (x) => (typeof x === "number" && Number.isFinite(x) ? x : 0);
  return {
    hour: num(facts.hour),
    weekday: num(facts.weekday),
    msSinceInput: num(facts.msSinceInput),
    selectedActiveMs: num(facts.selectedActiveMs),
    finishSeq: num(facts.finishSeq),
    recentFinishMs: Array.isArray(facts.recentFinishMs) ? facts.recentFinishMs.map(num) : [],
    firstRunOfDay: facts.firstRunOfDay === true,
    freshInstall: facts.freshInstall === true,
  };
}
