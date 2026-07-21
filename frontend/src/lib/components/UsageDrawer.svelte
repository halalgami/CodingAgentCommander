<script>
  import { onMount, onDestroy } from "svelte";
  import Drawer from "./Drawer.svelte";
  import { app, fetchPlanUsage } from "../stores.svelte.js";

  let usage = $state(null);
  let error = $state("");
  let loading = $state(false);
  let iv;

  async function refresh() {
    loading = true;
    try {
      usage = await fetchPlanUsage();
      error = "";
    } catch (e) { error = "" + e; }
    loading = false;
  }

  onMount(() => {
    refresh();
    iv = setInterval(refresh, 60_000); // limits move slowly; minute poll while open
  });
  onDestroy(() => clearInterval(iv));

  const session = $derived(usage?.windows.filter((w) => !w.weekly) ?? []);
  const weekly = $derived(usage?.windows.filter((w) => w.weekly) ?? []);

  function band(pct) {
    return pct >= 80 ? "red" : pct >= 50 ? "amber" : "green";
  }
  // 5h window -> countdown; weekly -> weekday + time.
  function resetText(w) {
    if (!w.resetsAt) return "";
    const t = new Date(w.resetsAt);
    if (isNaN(t)) return "";
    if (!w.weekly) {
      const mins = Math.max(0, Math.round((t - Date.now()) / 60000));
      return `Resets in ${Math.floor(mins / 60)} hr ${mins % 60} min`;
    }
    return "Resets " + t.toLocaleString(undefined, {
      weekday: "short", hour: "numeric", minute: "2-digit",
    });
  }
</script>

{#snippet windowRow(w)}
  <div class="win" data-testid="usage-window-{w.key}">
    <div class="head">
      <span class="label">{w.label}</span>
      <span class="pct mono">{Math.round(w.utilization)}% used</span>
    </div>
    <div class="meter" aria-hidden="true">
      <div class="fill {band(w.utilization)}" style="width: {Math.min(100, Math.max(1, w.utilization))}%"></div>
    </div>
    {#if resetText(w)}<div class="reset">{resetText(w)}</div>{/if}
  </div>
{/snippet}

<Drawer title="PLAN USAGE" testid="drawer-usage" onclose={() => (app.drawer = null)}>
  {#if error}
    <p class="err">{error}</p>
    <button data-testid="usage-retry" onclick={refresh}>Retry</button>
  {:else if !usage}
    <p class="dim">{loading ? "Fetching…" : ""}</p>
  {:else}
    {#each session as w (w.key)}{@render windowRow(w)}{/each}
    {#if weekly.length}
      <h3>Weekly limits</h3>
      {#each weekly as w (w.key)}{@render windowRow(w)}{/each}
    {/if}
    <p class="dim mono">as of {new Date(usage.fetchedAt).toLocaleTimeString()}</p>
  {/if}
</Drawer>

<style>
  .win { margin-bottom: var(--sp-4); }
  .head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: var(--sp-1); }
  .label { font-size: var(--fs-2); color: var(--text-0); }
  .pct { font-size: var(--fs-1); color: var(--text-1); }
  .meter {
    height: 4px; background: var(--surface-1); border-radius: 2px; overflow: hidden;
    border: 1px solid var(--border-0);
  }
  .fill { height: 100%; border-radius: 2px; transition: width var(--t-med) var(--ease-out); }
  .fill.green { background: var(--accent); }
  .fill.amber { background: var(--warn); }
  .fill.red { background: var(--crit); }
  .reset { margin-top: var(--sp-1); font-size: var(--fs-0); color: var(--text-2); }
  h3 { margin: var(--sp-4) 0 var(--sp-2); font-size: var(--fs-1); color: var(--text-1); text-transform: uppercase; letter-spacing: 0.08em; }
  .mono { font-family: var(--font-mono); }
  .dim { color: var(--text-2); font-size: var(--fs-1); }
  .err { color: var(--crit); font-size: var(--fs-1); }
  button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-1); padding: 4px 10px; cursor: pointer;
  }
</style>
