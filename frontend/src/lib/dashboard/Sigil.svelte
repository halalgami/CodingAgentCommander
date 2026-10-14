<script>
  import { sigil } from "./sigil.js";
  let { id, lead = undefined, tint = "var(--border-1)", size = 20 } = $props();
  const g = $derived(sigil(id, lead));
</script>

<svg class="sigil" width={size} height={size} viewBox="-1 -1 7 7" aria-hidden="true" style="--tint:{tint}">
  <rect x="-0.5" y="-0.5" width="6" height="6" rx="1.2" class="frame" />
  {#each g.cells as on, i}
    {#if on}<rect x={i % 5} y={Math.floor(i / 5)} width="1" height="1" fill="hsl({g.hue} 70% 60%)" />{/if}
  {/each}
</svg>

<style>
  .sigil { flex: none; display: block; }
  .frame { fill: var(--surface-2); stroke: var(--tint); stroke-width: 0.5; }
</style>
