// Binds the pure grouping module to the reactive prefs store. Separate from
// modelGroups.js so that stays node-testable, and separate from
// stores.svelte.js so the public export cannot drop it.
import { prefs, setPref } from "./prefs.svelte.js";
import { groupByProvider } from "./modelGroups.js";

// `needs key` is a picker-only annotation, so it is applied here rather than in
// the pure module — the drawer and the swap control do not want it.
export function pickerGroups(models) {
  return groupByProvider(models, { collapsed: prefs.collapsedProviders }).map((g) => ({
    ...g,
    models: g.models.map((m) => ({ ...m, suffix: m.routed && !m.ready ? " (needs key)" : "" })),
  }));
}

export function catalogGroups(models) {
  return groupByProvider(models, { collapsed: prefs.collapsedProviders });
}

// Non-mutating: always replaces the array (Object.freeze on DEFAULTS is shallow,
// so nothing else protects a shared one).
export function toggleProviderGroup(provider) {
  const now = prefs.collapsedProviders;
  setPref("collapsedProviders",
    now.includes(provider) ? now.filter((p) => p !== provider) : [...now, provider]);
}
