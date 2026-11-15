<script>
  // Quota card (spec §5): per provider window — current %, burn over the
  // window, and the pace forecast. Providers with no official endpoint say so.
  import { onMount } from "svelte";
  import { quota, refreshQuota, loadQuotaHistory } from "./quota.svelte.js";
  import { burnGeom } from "./geom.js";
  import { fmtForecast } from "./format.js";
  import Chart from "./Chart.svelte";

  let { seen = [] } = $props(); // provider ids with usage in the range
  const W = 220, H = 36;

  const history = () => { for (const p of quota.now?.providers || []) if (p.supported) loadQuotaHistory(p.provider); };
  // Checked once when the card opens, never on a timer (owner, 2026-11-15).
  onMount(() => { refreshQuota().then(history); });

  const supported = $derived((quota.now?.providers || []).filter((p) => p.supported));
  const unsupported = $derived((quota.now?.providers || []).filter((p) => !p.supported && seen.includes(p.provider)));

  function seriesOf(p) {
    const h = quota.history[p.provider];
    const now = Date.now();
    return (p.windows || []).map((w) => {
      const s = (h?.windows || []).find((x) => x.key === w.key);
      const pts = (s?.points || []).map((q) => ({ t: Date.parse(q.t), u: q.u }));
      const reset = Date.parse(w.resetsAt);
      const to = Number.isFinite(reset) ? reset : now;
      const from = Number.isFinite(reset) ? reset - (w.weekly ? 7 * 864e5 : 5 * 36e5) : (pts[0]?.t ?? now - 36e5);
      const fc = s?.forecast ? Date.parse(s.forecast) : null;
      return { w, pts, g: burnGeom(pts, from, to, W, H, fc), text: fmtForecast(s?.forecast || "", new Date(), s?.note || "") };
    });
  }
  const rows = $derived(supported.flatMap((p) => seriesOf(p).flatMap((x) => x.pts.map((q) => ({ win: `${p.label} · ${x.w.label}`, t: new Date(q.t).toLocaleString(), u: Math.round(q.u) })))));
  const locked = $derived(supported.some((p) => quota.history[p.provider]?.locked));
</script>

<Chart id="quota" title="Quota" sub="checked when you open this view" columns={[{ key: "win", label: "Window" }, { key: "t", label: "Time" }, { key: "u", label: "Used %", num: true }]} {rows}>
  <div class="quota" data-testid="ast-quota">
    {#each supported as p (p.provider)}
      {#if p.windows?.length}
        {#each seriesOf(p) as x (x.w.key)}
          <div class="qrow" data-testid="ast-quota-row" data-key={x.w.key}>
            <span>{p.label} · {x.w.label}</span>
            <span class="qbar"><i style:width={Math.min(100, x.w.util) + "%"} class:hot={x.w.util >= 80}></i></span>
            <span class="num">{Math.round(x.w.util)}%</span>
          </div>
          {#if !locked}
            <svg class="burn" viewBox="0 0 {W} {H}" width={W} height={H} aria-hidden="true">
              <line x1="0" y1="0" x2={W} y2="0" class="lim" />
              {#if x.g.path}<path d={x.g.path} class="line" />{/if}
              {#if x.g.forecast}<line {...x.g.forecast} class="fc" />{/if}
            </svg>
            <p class="small" data-testid="ast-quota-forecast">{x.text}</p>
          {/if}
        {/each}
      {:else}
        <p class="dim small" data-testid="ast-quota-error">{p.label}: {p.error || "no sample yet"}</p>
      {/if}
    {/each}
    {#if locked}<p class="dim small" data-testid="ast-quota-locked">Turn on history to keep quota history and see the forecast.</p>{/if}
    {#each unsupported as p (p.provider)}
      <div class="qrow none" data-testid="ast-quota-none"><span>{p.label}</span><span class="dim">No official quota</span></div>
    {/each}
    {#if !supported.length && !unsupported.length}<p class="dim small">No live quota available.</p>{/if}
  </div>
</Chart>

<style>
  .qrow { display: grid; grid-template-columns: 11em 1fr 3.5em; gap: var(--sp-2); align-items: center; font-size: var(--fs-0); color: var(--text-1); margin: 3px 0; }
  .qrow.none { grid-template-columns: 11em 1fr; }
  .qbar { height: 6px; background: var(--surface-2); border-radius: 3px; overflow: hidden; }
  .qbar i { display: block; height: 100%; background: var(--accent); border-radius: 3px; }
  .qbar i.hot { background: var(--warn); }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .burn { display: block; margin: 2px 0 0; }
  .burn .lim { stroke: var(--border-0); stroke-width: 1; stroke-dasharray: 2 3; }
  .burn .line { fill: none; stroke: var(--accent); stroke-width: 2; stroke-linejoin: round; }
  .burn .fc { stroke: var(--warn); stroke-width: 2; stroke-dasharray: 4 3; }
  .small { font-size: var(--fs-0); margin: 2px 0 6px; color: var(--text-1); }
  .dim { color: var(--text-2); }
</style>
