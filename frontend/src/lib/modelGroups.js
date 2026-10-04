// Pure provider-grouping for every model list in the app. No Svelte, no store
// access — node-testable, like prefsData.js and fuzzy.js.
//
// Deliberately NOT in stores.svelte.js: the public export overlays that file
// wholesale from scripts/_public-overrides/, and that copy runs behind main,
// so anything added there is silently dropped from the public mirror.

// Display names. Duplicated from stores.svelte.js's providerLabel on purpose —
// importing from stores would drag this module into the override problem above,
// and that copy has no "anthropic" entry (it only ever labelled the *routed*
// providers in the admin drawer), so it would render a lowercase header.
const LABELS = {
  anthropic: "Anthropic",
  "ollama-cloud": "Ollama Cloud",
  bedrock: "AWS Bedrock",
  "opencode-go": "OpenCode Zen/Go",
};

export function providerDisplayLabel(provider) {
  return LABELS[provider] ?? provider;
}

// Names a provider's rows are actually prefixed with, which are NOT the header
// label: discovery writes "Ollama · x" (internal/ollama), "Go · x"
// (internal/zen) and "Bedrock · x" (internal/bedrock), while the headers read
// "Ollama Cloud", "OpenCode Zen/Go" and "AWS Bedrock". The display label is
// always accepted too (hand-added or older catalog entries use it).
const STRIP_ALIASES = {
  anthropic: ["Anthropic"],
  "ollama-cloud": ["Ollama"],
  bedrock: ["Bedrock"],
  "opencode-go": ["Go"],
};

function prefixesFor(provider, providerLabel) {
  return [providerLabel, ...(STRIP_ALIASES[provider] ?? [])];
}

// Strip a leading "<Name> · " when Name is one of `names` (a string or list):
// the group's own display label or a known alias for its provider. Exact
// leading match only, case-insensitive: a hand-edited label, or one that
// mentions another provider, is left alone. Never empties the label.
export function stripProviderPrefix(label, names) {
  for (const name of [].concat(names)) {
    const prefix = name + " · ";
    if (label.slice(0, prefix.length).toLowerCase() !== prefix.toLowerCase()) continue;
    const rest = label.slice(prefix.length).trim();
    if (rest !== "") return rest;
  }
  return label;
}

// anthropic is pinned first: it is the native provider, always available, and
// needs no key. Everything else is alphabetical by DISPLAY label so the order
// matches what the user reads. Order is fixed rather than selection-dependent —
// groups that move under the user make the list unlearnable.
function compareProviders(a, b) {
  if (a.provider === b.provider) return 0;
  if (a.provider === "anthropic") return -1;
  if (b.provider === "anthropic") return 1;
  if (a.provider === "") return 1; // the Other bucket sinks
  if (b.provider === "") return -1;
  return a.label.localeCompare(b.label);
}

export function groupByProvider(models, { collapsed = [] } = {}) {
  // typeof [] === "object" and so does typeof null, so prefs round-tripping a
  // corrupt value must not reach .includes() on a non-array.
  const folded = Array.isArray(collapsed) ? collapsed : [];

  const byProvider = new Map();
  for (const m of models ?? []) {
    const provider = m.provider ?? "";
    if (!byProvider.has(provider)) byProvider.set(provider, []);
    byProvider.get(provider).push(m);
  }

  const groups = [...byProvider.entries()].map(([provider, list]) => ({
    provider,
    label: provider === "" ? "Other" : providerDisplayLabel(provider),
    models: list,
    count: list.length,
  }));
  groups.sort(compareProviders);

  const single = groups.length <= 1;
  return groups.map((g) => {
    return {
      ...g,
      single,
      // Every group folds, including the one holding the selection: the closed
      // trigger already names it, and scrolling past a big group costs more.
      collapsed: !single && folded.includes(g.provider),
      // With one provider there is no header to supply context, so the label
      // keeps its prefix.
      models: g.models.map((m) => ({
        ...m,
        displayLabel: single ? m.label : stripProviderPrefix(m.label, prefixesFor(g.provider, g.label)),
      })),
    };
  });
}
