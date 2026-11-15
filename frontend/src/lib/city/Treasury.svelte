<script>
  // Al Modon treasury (spec §6): each provider's quota and this week's spend.
  import { onMount } from "svelte";
  import { quota, refreshQuota } from "../asturlab/quota.svelte.js";
  onMount(() => { refreshQuota(); }); // checked when the treasury opens, never on a timer
  import { world } from "../asturlab/world.svelte.js";
  import { spendLabel } from "../asturlab/worldmath.js";

  const rows = $derived((quota.now?.providers || [])
    .filter((p) => p.supported || world.providers[p.provider])
    .map((p) => ({ ...p, spend: world.providers[p.provider], util: Math.max(0, ...(p.windows || []).map((w) => w.util)) })));
</script>

<aside class="treasury" data-testid="city-treasury" aria-label="Treasury">
  <h4>Treasury <span class="dim">last 7 days</span></h4>
  {#each rows as p (p.provider)}
    <div class="row" data-testid="city-treasury-row" data-provider={p.provider}>
      <span class="name">{p.label}</span>
      {#if p.supported && p.windows?.length}
        <span class="bar" title="Quota used {Math.round(p.util)}%"><i style:width={Math.min(100, p.util) + "%"} class:hot={p.util >= 80}></i></span>
        <span class="num">{Math.round(p.util)}%</span>
      {:else}
        <span class="dim none">{p.supported ? "no sample yet" : "No official quota"}</span>
      {/if}
      <span class="spend">{p.spend ? spendLabel(p.spend) : "—"}</span>
    </div>
  {/each}
  {#if world.pricedAt}<p class="dim small">$ ≈ API-equivalent, priced {world.pricedAt}{#if world.stale} · <span class="stale">rates may be stale</span>{/if}</p>{/if}
</aside>

<style>
  .treasury { position: absolute; right: 12px; top: 48px; width: 260px; background: rgba(12, 14, 20, 0.86); color: var(--text-0); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 8px 10px; font-size: var(--fs-0); z-index: 3; }
  h4 { margin: 0 0 6px; font-size: var(--fs-1); }
  .row { display: grid; grid-template-columns: 6.5em 1fr 2.8em; grid-template-areas: "n b p" "n s s"; gap: 2px 6px; align-items: center; margin: 4px 0; }
  .name { grid-area: n; } .bar, .none { grid-area: b; } .num { grid-area: p; text-align: right; font-variant-numeric: tabular-nums; }
  .none { grid-column: 2 / 4; }
  .spend { grid-area: s; color: var(--text-1); }
  .bar { height: 6px; background: var(--surface-2); border-radius: 3px; overflow: hidden; }
  .bar i { display: block; height: 100%; background: var(--accent); }
  .bar i.hot { background: var(--warn); }
  .dim { color: var(--text-2); } .small { margin: 6px 0 0; }
  .stale { color: var(--warn); }
</style>
