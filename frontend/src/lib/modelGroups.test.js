import { test } from "node:test";
import assert from "node:assert/strict";
import { groupByProvider, providerDisplayLabel, stripProviderPrefix } from "./modelGroups.js";

const M = (id, provider, label) => ({ id, provider, label: label ?? id });

test("anthropic leads, everything else is alphabetical by label", () => {
  const got = groupByProvider([
    M("g", "ollama-cloud"), M("z", "bedrock"), M("a", "anthropic"), M("k", "opencode-go"),
  ]);
  assert.deepEqual(got.map((x) => x.provider),
    ["anthropic", "bedrock", "ollama-cloud", "opencode-go"]);
});

test("order within a group is catalog order, never sorted", () => {
  const got = groupByProvider([M("zeta", "anthropic"), M("alpha", "anthropic")]);
  assert.deepEqual(got[0].models.map((m) => m.id), ["zeta", "alpha"]);
});

test("a single provider produces no group chrome", () => {
  const got = groupByProvider([M("a", "anthropic"), M("b", "anthropic")]);
  assert.equal(got.length, 1);
  assert.equal(got[0].single, true);
});

test("two providers are not single", () => {
  const got = groupByProvider([M("a", "anthropic"), M("b", "ollama-cloud")]);
  assert.equal(got.every((g) => g.single === false), true);
});

test("counts are reported even when collapsed", () => {
  const got = groupByProvider(
    [M("a", "anthropic"), M("b", "ollama-cloud"), M("c", "ollama-cloud")],
    { collapsed: ["ollama-cloud"] },
  );
  const oll = got.find((g) => g.provider === "ollama-cloud");
  assert.equal(oll.collapsed, true);
  assert.equal(oll.count, 2);
  // Models stay on the group so a header can report them; the renderer decides
  // whether to draw rows.
  assert.equal(oll.models.length, 2);
});

// There is no selection parameter: a collapsed group stays collapsed even when
// it holds the model that is currently selected (user decision 2026-10-04).
test("a collapsed list is honoured for every group, selected model or not", () => {
  const got = groupByProvider(
    [M("a", "anthropic"), M("b", "ollama-cloud")],
    { collapsed: ["ollama-cloud", "anthropic"] },
  );
  assert.equal(got.find((g) => g.provider === "ollama-cloud").collapsed, true);
  assert.equal(got.find((g) => g.provider === "anthropic").collapsed, true);
  assert.equal(got.every((g) => !("pinned" in g)), true);
});

// Multi-provider on purpose: with a single provider, `single` short-circuits
// the collapsed expression before `folded.includes()` is ever evaluated, so a
// single-provider fixture passes even with the Array.isArray guard deleted.
test("a corrupt collapsed list is ignored rather than throwing", () => {
  for (const bad of [null, undefined, "ollama-cloud", 7, {}]) {
    const got = groupByProvider([M("a", "anthropic"), M("b", "ollama-cloud")], { collapsed: bad });
    assert.equal(got.find((g) => g.provider === "ollama-cloud").collapsed, false);
    assert.equal(got.find((g) => g.provider === "anthropic").collapsed, false);
  }
});

test("known providers get display labels, unknown ones pass through", () => {
  assert.equal(providerDisplayLabel("anthropic"), "Anthropic");
  assert.equal(providerDisplayLabel("ollama-cloud"), "Ollama Cloud");
  assert.equal(providerDisplayLabel("bedrock"), "AWS Bedrock");
  assert.equal(providerDisplayLabel("opencode-go"), "OpenCode Zen/Go");
  assert.equal(providerDisplayLabel("made-up"), "made-up");
});

test("the provider prefix is stripped only when it matches the group", () => {
  assert.equal(stripProviderPrefix("Anthropic · Opus 5.5", "Anthropic"), "Opus 5.5");
  assert.equal(stripProviderPrefix("anthropic · Opus 5.5", "Anthropic"), "Opus 5.5");
  // A hand-edited label survives untouched.
  assert.equal(stripProviderPrefix("MY OPUS", "Anthropic"), "MY OPUS");
  // A label naming a DIFFERENT provider is not the group's prefix.
  assert.equal(stripProviderPrefix("Anthropic-tuned Qwen", "Ollama Cloud"), "Anthropic-tuned Qwen");
  // Partial word match must not trigger.
  assert.equal(stripProviderPrefix("Anthropics · X", "Anthropic"), "Anthropics · X");
  // Stripping must never empty the label.
  assert.equal(stripProviderPrefix("Anthropic · ", "Anthropic"), "Anthropic · ");
  // ...including a whitespace-only remainder.
  assert.equal(stripProviderPrefix("Anthropic ·    ", "Anthropic"), "Anthropic ·    ");
});

// REAL label formats, from internal/ollama, internal/zen, internal/bedrock.
test("each provider's real discovery prefix is stripped under its own group", () => {
  const got = groupByProvider([
    M("a", "anthropic", "Anthropic · Opus 5.5"),
    M("o", "ollama-cloud", "Ollama · glm-5.3"),
    M("o2", "ollama-cloud", "Ollama Cloud · qwen"),
    M("z", "opencode-go", "Go · kimi-k3"),
    M("z2", "opencode-go", "OpenCode Zen/Go · minimax"),
    M("b", "bedrock", "Bedrock · Anthropic Claude Opus"),
    M("b2", "bedrock", "AWS Bedrock · Llama"),
  ]);
  const labels = Object.fromEntries(got.flatMap((g) => g.models.map((m) => [m.id, m.displayLabel])));
  assert.deepEqual(labels, {
    a: "Opus 5.5", o: "glm-5.3", o2: "qwen", z: "kimi-k3", z2: "minimax",
    b: "Anthropic Claude Opus", b2: "Llama",
  });
});

test("an alias belongs to its own provider only", () => {
  const got = groupByProvider([
    M("a", "anthropic", "Anthropic · Opus"),
    M("o", "ollama-cloud", "Anthropic-tuned Qwen"),
    M("o2", "ollama-cloud", "Anthropic · borrowed"),
    M("b", "bedrock", "Ollama · odd"),
    M("g", "opencode-go", "Go-getter"),
  ]);
  const labels = Object.fromEntries(got.flatMap((g) => g.models.map((m) => [m.id, m.displayLabel])));
  assert.equal(labels.o, "Anthropic-tuned Qwen");
  assert.equal(labels.o2, "Anthropic · borrowed");
  assert.equal(labels.b, "Ollama · odd");
  assert.equal(labels.g, "Go-getter");
});

test("a hand-edited label under a routed group survives", () => {
  const got = groupByProvider([M("a", "anthropic"), M("o", "ollama-cloud", "My GLM")]);
  assert.equal(got.find((g) => g.provider === "ollama-cloud").models[0].displayLabel, "My GLM");
});

test("displayLabel is stripped in a group but the raw label is preserved", () => {
  const got = groupByProvider([
    M("o", "anthropic", "Anthropic · Opus 5.5"), M("g", "ollama-cloud", "Ollama · glm"),
  ]);
  const a = got.find((g) => g.provider === "anthropic").models[0];
  assert.equal(a.displayLabel, "Opus 5.5");
  assert.equal(a.label, "Anthropic · Opus 5.5", "the payload label must not be mutated");
});

test("a single-provider catalog keeps full labels, since no header supplies context", () => {
  const got = groupByProvider([M("o", "anthropic", "Anthropic · Opus 5.5")]);
  assert.equal(got[0].models[0].displayLabel, "Anthropic · Opus 5.5");
});

test("models with no provider collect under an Other group, last", () => {
  const got = groupByProvider([M("x", undefined), M("a", "anthropic")]);
  assert.deepEqual(got.map((g) => g.provider), ["anthropic", ""]);
  assert.equal(got[1].label, "Other");
});

test("every group collapsed: all collapsed, counts intact", () => {
  const got = groupByProvider(
    [M("a", "anthropic"), M("a2", "anthropic"), M("b", "ollama-cloud")],
    { collapsed: ["anthropic", "ollama-cloud"] },
  );
  assert.deepEqual(got.map((g) => g.collapsed), [true, true]);
  assert.deepEqual(got.map((g) => g.count), [2, 1]);
});
