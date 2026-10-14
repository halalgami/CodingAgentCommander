<script>
  import { flip } from "svelte/animate";
  import Tile from "./Tile.svelte";
  import { bandOf, density } from "./rows.js";
  let { rows, timeline, now, onopen } = $props();
  const bands = [
    { id: "needs", label: "Needs you" },
    { id: "working", label: "Working" },
    { id: "quiet", label: "Quiet" },
  ];
  const d = $derived(density(rows.length));
  const tagged = $derived(rows.map((r) => ({ ...r, band: bandOf(r) })));
</script>

<div class="wall {d}" data-testid="dash-triage">
  {#each bands as b (b.id)}
    {@const items = tagged.filter((r) => r.band === b.id)}
    {#if items.length}
      <section class="band" data-testid="dash-band-{b.id}">
        <h2>{b.label} <span>{items.length}</span></h2>
        <div class="grid">
          {#each items as row (row.key)}
            <div animate:flip={{ duration: 260 }}>
              <Tile {row} {now} density={d} events={timeline[row.id] || []} {onopen} />
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/each}
</div>

<style>
  .wall { padding: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-4); }
  h2 { font-size: var(--fs-1); color: var(--text-2); font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; margin: 0 0 var(--sp-2); }
  h2 span { color: var(--text-1); }
  .grid { display: grid; gap: var(--sp-3); grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); }
  .wall.compact .grid { grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: var(--sp-2); }
  .wall.hero .grid { grid-template-columns: minmax(0, 640px); justify-content: center; }
</style>
