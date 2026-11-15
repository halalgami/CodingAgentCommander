<script>
  // Settings → Al Asturlab: plan prices (value ×) and per-model rate overrides.
  import { onMount } from "svelte";
  import { AsturlabSettings, SetAsturlabSettings, Stats } from "../../../wailsjs/go/main/App.js";
  import { refresh } from "../asturlab/asturlab.svelte.js";

  const PROVIDERS = [["anthropic", "Anthropic"], ["bedrock", "Bedrock"], ["ollama-cloud", "Ollama Cloud"], ["opencode-go", "OpenCode Go"]];
  let plans = $state(Object.fromEntries(PROVIDERS.map(([id]) => [id, { name: "", usd: "" }])));
  let overrides = $state([]); // [{ model, in, out, cacheRead, cacheWrite }]
  let draft = $state({ model: "", in: "", out: "", cacheRead: "", cacheWrite: "" });
  let pricedAt = $state(""), stale = $state(false), msg = $state("");

  onMount(async () => {
    try {
      const s = (await AsturlabSettings()) || {};
      for (const [id, p] of Object.entries(s.plans || {})) plans[id] = { name: p.name || "", usd: String(p.monthlyUSD ?? "") };
      overrides = Object.entries(s.overrides || {}).map(([model, r]) => ({ model, ...r }));
    } catch {}
    try { const r = await Stats({ groupBy: "provider", metric: "tokens" }); pricedAt = r.pricedAt; stale = r.stale; } catch {}
  });

  const num = (v) => { const n = parseFloat(v); return Number.isFinite(n) && n >= 0 ? n : 0; };
  async function save() {
    const out = { plans: {}, overrides: {} };
    for (const [id, p] of Object.entries(plans)) if (num(p.usd) > 0) out.plans[id] = { name: p.name.trim(), monthlyUSD: num(p.usd) };
    const rows = [...overrides];
    if (draft.model.trim()) rows.push({ ...draft, model: draft.model.trim() });
    // A blank rate would price the model at $0 and hide its tokens as "priced".
    const bad = rows.find((r) => !(num(r.in) > 0 || num(r.out) > 0));
    if (bad) { msg = `${bad.model} needs an in or out rate.`; return; }
    for (const r of rows) out.overrides[r.model] = { in: num(r.in), out: num(r.out), cacheRead: num(r.cacheRead), cacheWrite: num(r.cacheWrite) };
    try {
      await SetAsturlabSettings(out);
      overrides = Object.entries(out.overrides).map(([model, r]) => ({ model, ...r }));
      draft = { model: "", in: "", out: "", cacheRead: "", cacheWrite: "" };
      msg = "Saved.";
      refresh();
    } catch (e) { msg = "Couldn't save: " + e; }
  }
</script>

<section data-testid="ast-settings">
  <h3>Al Asturlab</h3>
  <p class="dim">Dollar figures are API-equivalent estimates from a rate table priced {pricedAt || "—"}{#if stale} <span class="stale">(may be stale)</span>{/if}.
    Models without a rate show tokens only.</p>
  <h4>Plans</h4>
  {#each PROVIDERS as [id, label]}
    <div class="row">
      <span class="lbl">{label}</span>
      <input placeholder="Plan name" data-testid={"ast-plan-" + id + "-name"} bind:value={plans[id].name} />
      <input placeholder="$ / month" inputmode="decimal" data-testid={"ast-plan-" + id + "-usd"} bind:value={plans[id].usd} />
    </div>
  {/each}
  <h4>Rate overrides <span class="dim">USD per million tokens · model id or provider:&lt;id&gt;</span></h4>
  {#each overrides as r, i}
    <div class="row rate">
      <span class="lbl mono">{r.model}</span>
      <input aria-label="in" bind:value={r.in} /><input aria-label="out" bind:value={r.out} />
      <input aria-label="cache read" bind:value={r.cacheRead} /><input aria-label="cache write" bind:value={r.cacheWrite} />
      <button class="ghost" onclick={() => (overrides = overrides.filter((_, j) => j !== i))}>Remove</button>
    </div>
  {/each}
  <div class="row rate">
    <input class="lbl" placeholder="model id" data-testid="ast-rate-model" bind:value={draft.model} />
    <input placeholder="in" data-testid="ast-rate-in" bind:value={draft.in} /><input placeholder="out" data-testid="ast-rate-out" bind:value={draft.out} />
    <input placeholder="cache read" data-testid="ast-rate-read" bind:value={draft.cacheRead} /><input placeholder="cache write" data-testid="ast-rate-write" bind:value={draft.cacheWrite} />
  </div>
  <div class="row"><button class="ghost" data-testid="ast-settings-save" onclick={save}>Save</button><span class="dim">{msg}</span></div>
</section>

<style>
  .row { display: flex; gap: var(--sp-2); align-items: center; margin: var(--sp-1) 0; }
  .lbl { width: 9em; flex: none; font-size: var(--fs-1); color: var(--text-1); overflow: hidden; text-overflow: ellipsis; }
  input { background: var(--surface-2); border: 1px solid var(--border-0); color: var(--text-0); border-radius: var(--r-1); padding: 2px 6px; min-width: 0; width: 7em; }
  .rate input:not(.lbl) { width: 4.5em; }
  .dim { color: var(--text-1); font-size: var(--fs-1); }
  .stale { color: var(--warn); }
  .mono { font-family: var(--font-mono); }
  h4 { font-size: var(--fs-1); color: var(--text-1); margin: var(--sp-3) 0 var(--sp-1); }
  .ghost { background: var(--surface-2); color: var(--text-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 2px 8px; cursor: pointer; }
</style>
