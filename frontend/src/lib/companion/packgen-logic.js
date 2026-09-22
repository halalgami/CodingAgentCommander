// The wizard's decisions, as pure functions.
//
// Deliberately free of runes, Wails imports and DOM: everything here is
// testable with `node --test`, which the reactive shell in packgen.svelte.js
// is not — `$state` is a compiler rune and does not exist in plain node.
// Anything that decides something belongs in this file; packgen.svelte.js
// holds state and makes calls.

/** The order phases appear in. */
export const PHASES = ["base", "slots", "generate", "review"];

/** Composite key for a pick: a slot, optionally narrowed by a condition. */
export function slotKey(slot, when) {
  return when ? `${slot}|${when}` : slot;
}

/**
 * The phase to show, given Go's run state and the step the user last chose.
 *
 * The run WINS wherever it has an opinion. A wizard reopened mid-run must land
 * on the grid, not on step 1 with a Generate button over a run that is already
 * spending money.
 */
export function phaseFor(run, step) {
  if (run?.status === "running") return "generate";
  // Finished but unsaved art needs a decision: those images are paid for.
  if (run && run.status !== "running" && !run.saved && runHoldsArt(run)) return "review";
  // A stale step would otherwise render a run phase with no run behind it —
  // an empty grid above a Save button.
  return step === "generate" || step === "review" ? "base" : step;
}

export function hasArt(run) {
  return (run?.items ?? []).some((i) => i.status === "done");
}

/** The ticked picks, in the shape Go's PackGenRequest expects. */
export function selectedSlotsFrom(picks) {
  return Object.entries(picks ?? {})
    .filter(([, v]) => v?.on)
    .map(([key, v]) => {
      const [slot, when = ""] = key.split("|");
      return { slot, when, staging: v.staging ?? "" };
    });
}

/** Every base slot on, with its default staging; conditions start off. */
export function defaultPicks(slotDefaults) {
  const picks = {};
  for (const s of slotDefaults ?? []) {
    picks[s.slot] = { on: true, staging: s.staging ?? "" };
  }
  return picks;
}

/** Conditions Go says can actually display in this slot. */
export function conditionsFor(conditions, slot) {
  return (conditions ?? []).filter((c) => (c.slots ?? []).includes(slot));
}

/**
 * The pack's most-common recorded model, so an added scene stays the same
 * character. Ties resolve to the FIRST model seen (strict > keeps the earliest),
 * which is deterministic. Falsy models are skipped: variant.model is omitempty
 * and arrives undefined for hand-authored packs. "" when nothing is recorded.
 */
export function packModel(browse) {
  const counts = new Map();
  let best = "", bestN = 0;
  for (const v of browse?.variants ?? []) {
    const m = v?.model;
    if (!m) continue;
    const n = (counts.get(m) ?? 0) + 1;
    counts.set(m, n);
    if (n > bestN) { bestN = n; best = m; }
  }
  return best;
}

/**
 * One row per slot and per allowed condition, for the "Add scenes" picker.
 * `present` is keyed on slot|when — deliberately broader than record()'s
 * slot|when|mood — so a saved variant at any mood blocks the whole slot|when
 * from being added, which is what stops a duplicate scene being generated.
 */
export function addableScenes(slotDefaults, conditions, browse) {
  const present = new Set(
    (browse?.variants ?? []).map((v) => slotKey(v.slot, v.when || "")),
  );
  const rows = [];
  for (const s of slotDefaults ?? []) {
    rows.push({ slot: s.slot, when: "", label: s.slot, present: present.has(slotKey(s.slot, "")) });
    for (const c of conditionsFor(conditions, s.slot)) {
      rows.push({
        slot: s.slot, when: c.name, label: `${s.slot} · ${c.name}`,
        present: present.has(slotKey(s.slot, c.name)),
      });
    }
  }
  return rows;
}

/**
 * Ticked add-picks in Go's PackGenSlot shape, excluding any combo already in
 * the pack at any mood. `present` is a Set (or iterable) of slot|when keys.
 */
export function selectedAddSlots(addPicks, present) {
  const presentSet = present instanceof Set ? present : new Set(present ?? []);
  return Object.entries(addPicks ?? {})
    .filter(([, v]) => v?.on)
    .map(([key, v]) => {
      const [slot, when = ""] = key.split("|");
      return { slot, when, staging: v.staging ?? "" };
    })
    .filter((s) => !presentSet.has(slotKey(s.slot, s.when)));
}

/**
 * Whether a run holds unsaved art. Trusts the run's own hasArt (Go's truth:
 * len(arts) > 0) when present, so a seeded regenerate/add run that finished
 * with every requested scene failed is still recognised as holding art. Falls
 * back to the done-item count for callers/tests that predate the field.
 */
export function runHoldsArt(run) {
  if (!run) return false;
  return run.hasArt ?? hasArt(run);
}

/**
 * A model belonging to the chosen provider. Carrying a model across a provider
 * switch would be rejected by Go, but only after a whole run was configured.
 * Returns "" rather than undefined — `undefined` reaches Go as the string
 * "undefined" and produces a baffling error.
 */
export function nextModel(providers, providerID, current) {
  const models = (providers ?? []).find((p) => p.id === providerID)?.models ?? [];
  return models.some((m) => m.id === current) ? current : (models[0]?.id ?? "");
}

/**
 * Why Generate is unavailable, as a sentence, or "" when it is available.
 *
 * Returned as text rather than a boolean on purpose: a disabled button with no
 * explanation is the most common way a wizard dead-ends, and every one of
 * these conditions has a specific fix the user can act on.
 */
export function blockerFor({ name, base, provider, model, picks, run, conditions }) {
  const selected = selectedSlotsFrom(picks);
  if (!String(name ?? "").trim()) return "Give the pack a name.";
  if (!base) return "Choose a base portrait.";
  if (!provider) return "No image provider is available.";
  if (!provider.hasKey) return `Add an API key for ${provider.id}.`;
  if (!model) return "Choose a model.";
  if (!selected.length) return "Pick at least one scene.";
  // Mirrors validatePackGenSlots: a pack without idle warns on every load, and
  // a CONDITIONED idle does not satisfy the requirement.
  if (!selected.some((s) => s.slot === "idle" && !s.when)) {
    return "The idle scene is required — every pack needs one.";
  }
  // Two ambient conditions on one slot can both hold at once, and the loader
  // warns. Caught here so the user is told while choosing rather than refused
  // after pressing Generate.
  const classOf = {};
  for (const c of conditions ?? []) classOf[c.name] = c.class;
  const ambientBySlot = {};
  for (const s of selected) {
    // Unknown class is treated as ambient: the conservative direction is to
    // warn about a pairing that might be fine, not to stay silent about one
    // that produces art the user cannot see.
    if (!s.when || classOf[s.when] === "event") continue;
    (ambientBySlot[s.slot] ??= new Set()).add(s.when);
  }
  for (const [slot, set] of Object.entries(ambientBySlot)) {
    if (set.size > 1) {
      return `${slot} has two conditions that can both be true at once (${[...set].sort().join(", ")}) — pick one.`;
    }
  }
  if (run?.status === "running") return "A pack is already generating.";
  if (run && !run.saved && runHoldsArt(run)) return "Save or discard the previous images first.";
  return "";
}

/** What to show under a card, given its item status. */
export function statusText(item) {
  switch (item?.status) {
    case "failed":  return item.error || "failed";
    case "skipped": return "skipped";
    case "running": return "generating…";
    case "pending": return "queued";
    default:        return "";
  }
}

/**
 * The cost line. Known=false must produce a COUNT and never a figure: a stale
 * or absent price rendered as $0.00 reads as free.
 */
export function costLine(n, cost) {
  const images = `${n} image${n === 1 ? "" : "s"}`;
  if (!cost?.known) return { images, price: "", note: "price unknown for this model" };
  return { images, price: `$${cost.usd.toFixed(2)}`, note: cost.note ?? "" };
}
