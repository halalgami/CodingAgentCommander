<script>
  // Frame shared by every Al Asturlab chart: title, the SVG (children), a hover
  // tooltip, and a keyboard-reachable table of the same rows (a11y fallback).
  let { id, title, sub = "", columns = [], rows = [], tip = null, onexport = null, wide = false, children, legend = null } = $props();
  let table = $state(false);
</script>

<section class="chart" class:wide data-testid={"ast-chart-" + id} data-chart={id} aria-label={title}>
  <header>
    <h3>{title}</h3>
    {#if sub}<span class="sub">{sub}</span>{/if}
    <span class="tools">
      <button class="link" data-testid="ast-table-toggle" aria-pressed={table} onclick={() => (table = !table)}>{table ? "Chart" : "Table"}</button>
      {#if onexport}<button class="link" data-testid="ast-chart-export" onclick={onexport}>Export</button>{/if}
    </span>
  </header>
  {#if legend}{@render legend()}{/if}
  {#if table}
    <div class="tbl">
      <table>
        <thead><tr>{#each columns as c}<th scope="col">{c.label}</th>{/each}</tr></thead>
        <tbody>
          {#each rows as r}<tr>{#each columns as c}<td class:num={c.num}>{c.fmt ? c.fmt(r) : r[c.key]}</td>{/each}</tr>{/each}
          {#if !rows.length}<tr><td colspan={columns.length} class="dim">No data in this range.</td></tr>{/if}
        </tbody>
      </table>
    </div>
  {:else}
    <div class="plot">
      {@render children()}
      {#if tip}<div class="tip" data-testid="ast-tip" role="status" style:left={tip.x + "px"} style:top={tip.y + "px"}>{#each tip.lines as l}<div>{l}</div>{/each}</div>{/if}
    </div>
  {/if}
</section>

<style>
  .chart { background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-3); min-width: 0; }
  header { display: flex; align-items: baseline; gap: var(--sp-2); margin-bottom: var(--sp-2); }
  h3 { margin: 0; font-size: var(--fs-1); font-weight: 600; color: var(--text-0); }
  .sub { color: var(--text-2); font-size: var(--fs-0); }
  .tools { margin-left: auto; display: flex; gap: var(--sp-2); }
  .link { background: none; border: 0; color: var(--text-1); font-size: var(--fs-0); cursor: pointer; padding: 2px 4px; border-radius: var(--r-1); }
  .link:hover, .link:focus-visible { color: var(--text-0); background: var(--surface-2); }
  .plot { position: relative; }
  .tip { position: absolute; pointer-events: none; transform: translate(-50%, calc(-100% - 8px)); background: var(--surface-3); color: var(--text-0);
    border: 1px solid var(--border-1); border-radius: var(--r-1); padding: 4px 8px; font-size: var(--fs-0); white-space: nowrap; z-index: 2; font-variant-numeric: tabular-nums; }
  .tbl { max-height: 260px; overflow: auto; }
  table { width: 100%; border-collapse: collapse; font-size: var(--fs-0); }
  th { text-align: left; color: var(--text-2); font-weight: 500; position: sticky; top: 0; background: var(--surface-1); }
  th, td { padding: 3px 6px; border-bottom: 1px solid var(--border-0); }
  td { color: var(--text-1); }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .dim { color: var(--text-2); }
</style>
