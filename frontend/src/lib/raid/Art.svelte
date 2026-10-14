<!-- frontend/src/lib/raid/Art.svelte — a bundled image (or the first that loads from a list), else a tinted silhouette (or nothing, when quiet). -->
<script>
  let { src, tint = "#888", cls = "", testid = undefined, quiet = false } = $props(); // quiet: nothing instead of a silhouette
  const list = $derived((Array.isArray(src) ? src : [src]).filter(Boolean));
  let i = $state(0);
  const key = $derived(list.join("|")); // content, not identity: callers build a new array every tick
  $effect(() => { void key; i = 0; }); // a new src gets a fresh try
</script>

{#if i < list.length}
  <img class="art {cls}" src={list[i]} alt="" draggable="false" data-testid={testid} onerror={() => (i += 1)} />
{:else if !quiet}
  <span class="art sil {cls}" style="--tint:{tint}" data-testid={testid ? testid + "-sil" : "raid-sil"}></span>
{/if}

<style>
  .art { display: block; max-width: 100%; max-height: 100%; object-fit: contain; user-select: none; }
  .sil { width: 60%; height: 80%; margin: auto; border-radius: 40% 40% 12% 12%; background: color-mix(in srgb, var(--tint) 55%, transparent); }
</style>
