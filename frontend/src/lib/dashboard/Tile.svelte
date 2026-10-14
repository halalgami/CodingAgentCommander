<script>
  import Sigil from "./Sigil.svelte";
  import CrewStrip from "./CrewStrip.svelte";
  import { activityText } from "../activity.js";
  import { labelOf } from "./label.js";
  import { fmtDuration, sparkline } from "./river.js";
  let { row, now, events = [], density = "standard", onopen } = $props();
  const act = $derived(activityText({ activity: row.activity, tool: row.tool, target: row.target, hotspot: row.hotspot,
    unread: row.unread, delegating: row.delegating, crewOut: row.crewOut }));
  const label = $derived(labelOf(row));
  const spark = $derived(sparkline(events, now, 10 * 60 * 1000, 20));
  const peak = $derived(Math.max(1, ...spark));
  const action = $derived(row.external ? "Details" : row.activity === "permission" ? "Review →" : "Open");
  let details = $state(false);
  let el;
</script>

<article bind:this={el} class="tile {density} st-{row.activity}" class:external={row.external}
  data-testid="dash-tile" data-key={row.key} data-band={row.band}>
  <header>
    <Sigil id={row.id || row.key} tint={row.tint} size={density === "compact" ? 16 : 22} />
    <span class="project" data-testid="tile-project">{label.project}</span>
    {#if row.model}<span class="chip">{row.model}</span>{/if}
    {#if row.external}<span class="chip ext" data-testid="external-badge">external</span>{/if}
  </header>
  <!-- Untitled: the activity is the task line, not repeated below. -->
  <div class="title" class:untitled={!label.titled} data-testid="tile-title">{label.titled ? label.task : act}</div>
  {#if label.titled}<div class="activity" data-testid="tile-activity">{#key act}<span class="slide">{act}</span>{/key}</div>{/if}
  {#if row.crew.length}<CrewStrip lead={row.id} crew={row.crew} compact={density === "compact"} />{/if}
  {#if density !== "compact"}
    <footer>
      {#if row.ctxPct != null}<span class="ctx" title="context {row.ctxPct}%"><span class="fill {row.ctxBand}" style="width:{row.ctxPct}%"></span></span>{/if}
      {#if row.turns != null}<span>{row.turns} turns</span>{/if}
      {#if row.sinceMs}<span>{fmtDuration(now - row.sinceMs)}</span>{/if}
      {#if row.rank}<span class="chip" data-testid="tile-rank">{row.rank}</span>{/if}
      {#if row.hero}<span class="chip" data-testid="tile-hero">L{row.hero.level} {row.hero.title}</span>{/if}
      <svg class="spark" viewBox="0 0 20 6" preserveAspectRatio="none" aria-hidden="true">
        {#each spark as v, i}<rect x={i} y={6 - (v / peak) * 6} width="0.8" height={(v / peak) * 6} />{/each}
      </svg>
    </footer>
  {/if}
  <button class="act" data-testid="tile-action" data-nav
    onclick={() => (row.external ? (details = !details) : onopen(row, el))}>{action}</button>
  {#if details}
    <dl class="details" data-testid="tile-details">
      <dt>Folder</dt><dd>{row.cwd}</dd>
      <dt>Model</dt><dd>{row.model || "—"}</dd>
      <dt>State</dt><dd>{act || row.activity}</dd>
      <dt>Crew</dt><dd>{row.crew.length}</dd>
    </dl>
  {/if}
</article>

<style>
  .tile {
    position: relative; display: flex; flex-direction: column; gap: 6px; padding: var(--sp-3);
    background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-3); min-width: 0;
  }
  .tile.hero { padding: var(--sp-5); font-size: var(--fs-3); }
  .tile.compact { padding: var(--sp-2); gap: 3px; }
  .tile.external { opacity: 0.7; }
  .tile.st-permission { border-color: var(--warn); animation: breathe 2.4s ease-in-out infinite; }
  .tile.st-error { border-color: var(--crit); animation: shake 300ms 1; }
  .tile.st-finished { animation: bloom 700ms var(--ease-out) 1; }
  header { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .project { color: var(--text-0); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .chip { font-size: var(--fs-0); color: var(--text-1); background: var(--surface-2); border-radius: var(--r-1); padding: 0 5px; white-space: nowrap; }
  .chip.ext { color: var(--text-2); border: 1px dashed var(--border-1); }
  .title { color: var(--text-1); overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
  .title.untitled { font-family: var(--font-mono); font-size: var(--fs-1); }
  .activity { color: var(--text-1); font-family: var(--font-mono); font-size: var(--fs-1); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .st-thinking .activity, .st-tool .activity { animation: shimmer 3s ease-in-out infinite; }
  .slide { display: inline-block; animation: slide-in 220ms var(--ease-out); }
  footer { display: flex; align-items: center; gap: 8px; color: var(--text-2); font-size: var(--fs-0); flex-wrap: wrap; }
  .ctx { width: 48px; height: 4px; background: var(--surface-3); border-radius: 2px; overflow: hidden; }
  .fill { display: block; height: 100%; }
  .fill.green { background: var(--ok); } .fill.amber { background: var(--warn); } .fill.red { background: var(--crit); }
  .spark { width: 60px; height: 14px; margin-left: auto; fill: var(--accent-dim); }
  .act {
    align-self: flex-start; background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-1);
    border-radius: var(--r-2); padding: 2px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  .st-permission .act { border-color: var(--warn); }
  .details { display: grid; grid-template-columns: auto 1fr; gap: 2px 8px; font-size: var(--fs-0); color: var(--text-1); margin: 0; }
  .details dd { margin: 0; overflow-wrap: anywhere; }
  @keyframes breathe { 50% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--warn) 35%, transparent); } }
  @keyframes shake { 25% { transform: translateX(-3px); } 75% { transform: translateX(3px); } }
  @keyframes bloom { from { box-shadow: 0 0 0 6px var(--accent-faint); } }
  @keyframes shimmer { 50% { opacity: 0.65; } }
  @keyframes slide-in { from { transform: translateY(6px); opacity: 0; } }
</style>
