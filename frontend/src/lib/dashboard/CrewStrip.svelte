<script>
  import Sigil from "./Sigil.svelte";
  import { crewView } from "./rows.js";
  let { lead, crew, compact = false } = $props();
  const v = $derived(crewView(crew));
  const tip = (a) => `${a.type || "agent"} · ${a.description || "(no description)"} — ${a.tool ? a.tool + " " + (a.target || "") : a.state}`;
</script>

{#if compact}
  <div class="crew compact">⑂ {crew.length}</div>
{:else}
  <div class="crew" data-testid="crew-strip">
    {#each v.shown as a (a.id)}
      <span class="chip {a.state}" class:bg={a.background} data-testid="crew-chip" title={tip(a)}>
        <Sigil id={a.id} {lead} size={14} />
      </span>
    {/each}
    {#if v.more > 0}<span class="more" data-testid="crew-more">+{v.more}</span>{/if}
  </div>
{/if}

<style>
  .crew { display: flex; gap: 3px; align-items: center; min-height: 18px; }
  .crew.compact { font-size: var(--fs-0); color: var(--text-2); }
  .chip { display: inline-flex; padding: 1px; border-radius: var(--r-1); border: 1px solid var(--border-0); }
  .chip.bg { border-style: dashed; }
  .chip.working, .chip.tool { animation: crew-shimmer 2.4s ease-in-out infinite; }
  .chip.finished { animation: crew-bloom 600ms var(--ease-out) 1; opacity: 0.6; }
  .chip.failed { border-color: var(--crit); }
  .chip.lost { opacity: 0.35; }
  .more { font-size: var(--fs-0); color: var(--text-2); padding: 0 4px; }
  @keyframes crew-shimmer { 50% { border-color: var(--accent-dim); } }
  @keyframes crew-bloom { from { transform: scale(1.4); opacity: 1; } }
</style>
