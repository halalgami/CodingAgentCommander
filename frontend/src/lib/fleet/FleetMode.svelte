<!-- frontend/src/lib/fleet/FleetMode.svelte — Fleet Ops (phase 7a spec §5): bands × panels, as Raid. -->
<script>
  import { flip } from "svelte/animate";
  import FleetPanel from "./FleetPanel.svelte";
  import { bandOf, density } from "../dashboard/rows.js";
  let { rows, timeline, projects = [], now, animate = true, onopen } = $props();
  const bands = [
    { id: "needs", label: "Awaiting orders" },
    { id: "working", label: "Engaged" },
    { id: "quiet", label: "Holding" },
  ];
  const d = $derived(density(rows.length));
  const hour = $derived(new Date(now).getHours());
  const latestOf = (key) => projects.find((p) => p.key === key)?.latest || null;
  const tagged = $derived(rows.map((r) => ({ ...r, band: bandOf(r) })));
</script>

<div class="fleet {d}" data-testid="dash-fleet">
  {#each bands as b (b.id)}
    {@const items = tagged.filter((r) => r.band === b.id)}
    {#if items.length}
      <section class="band" data-testid="fleet-band-{b.id}">
        <h2>{b.label} <span>{items.length}</span></h2>
        <div class="grid">
          {#each items as row (row.key)}
            <div animate:flip={{ duration: 260 }}>
              <FleetPanel {row} latest={latestOf(row.projectKey)} events={timeline[row.id] || []} {now} {hour} {animate} density={d} {onopen} />
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/each}
</div>

<style>
  .fleet { padding: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-4); }
  h2 { font-size: var(--fs-1); color: var(--text-2); font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; margin: 0 0 var(--sp-2); }
  h2 span { color: var(--text-1); }
  .grid { display: grid; gap: var(--sp-3); grid-template-columns: repeat(auto-fill, minmax(380px, 1fr)); }
  .fleet.compact .grid { grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: var(--sp-2); }
  .fleet.hero .grid { grid-template-columns: minmax(0, 960px); justify-content: center; }
</style>
