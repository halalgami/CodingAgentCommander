<!-- frontend/src/lib/raid/RaidMode.svelte -->
<script>
  import { flip } from "svelte/animate";
  import RaidCard from "./RaidCard.svelte";
  import { bandOf, density } from "../dashboard/rows.js";
  import { onMount } from "svelte";
  import { app, fetchPlanUsage } from "../stores.svelte.js";
  import { quotaOf } from "./looks.js";
  let { rows, timeline, projects = [], progress = null, now, animate = true, onopen } = $props();
  // The gold pouch reads account quota; fetch on entry (and every 5 min) so it shows without opening the usage drawer.
  onMount(() => {
    const load = () => fetchPlanUsage().then((u) => { const q = quotaOf(u); if (q != null) app.quotaPct = q; }).catch(() => {});
    load();
    const iv = setInterval(load, 300000);
    return () => clearInterval(iv);
  });
  const bands = [
    { id: "needs", label: "Needs you" },
    { id: "working", label: "Working" },
    { id: "quiet", label: "Quiet" },
  ];
  const d = $derived(density(rows.length));
  const hour = $derived(new Date(now).getHours());
  const latestOf = (key) => projects.find((p) => p.key === key)?.latest || null;
  const tagged = $derived(rows.map((r) => ({ ...r, band: bandOf(r) })));
</script>

<div class="raid {d}" data-testid="dash-raid">
  {#each bands as b (b.id)}
    {@const items = tagged.filter((r) => r.band === b.id)}
    {#if items.length}
      <section class="band" data-testid="raid-band-{b.id}">
        <h2>{b.label} <span>{items.length}</span></h2>
        <div class="grid">
          {#each items as row (row.key)}
            <div animate:flip={{ duration: 260 }}>
              <RaidCard {row} latest={latestOf(row.projectKey)} events={timeline[row.id] || []} {now} {hour} {animate} density={d} {progress} quotaPct={app.quotaPct} {onopen} />
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/each}
</div>

<style>
  .raid { padding: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-4); }
  h2 { font-size: var(--fs-1); color: var(--text-2); font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; margin: 0 0 var(--sp-2); }
  h2 span { color: var(--text-1); }
  .grid { display: grid; gap: var(--sp-3); grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); }
  .raid.compact .grid { grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: var(--sp-2); }
  .raid.hero .grid { grid-template-columns: minmax(0, 860px); justify-content: center; }
</style>
