<script>
  let { options = [], value = $bindable(""), placeholder = "select…", testid, onchange } = $props();
  let open = $state(false);
  let root = $state(null);
  let hi = $state(0);

  const current = $derived(options.find((o) => o.value === value));

  function choose(o) {
    value = o.value;
    open = false;
    onchange?.(o.value);
  }
  function onkeydown(e) {
    if (!open && (e.key === "Enter" || e.key === " " || e.key === "ArrowDown")) { e.preventDefault(); open = true; hi = 0; return; }
    if (!open) return;
    if (e.key === "Escape") { e.stopPropagation(); open = false; }
    if (e.key === "ArrowDown") { e.preventDefault(); hi = Math.min(hi + 1, options.length - 1); }
    if (e.key === "ArrowUp") { e.preventDefault(); hi = Math.max(hi - 1, 0); }
    if (e.key === "Enter") { e.preventDefault(); choose(options[hi]); }
  }
  function outside(e) {
    if (root && !root.contains(e.target)) open = false;
  }
</script>

<svelte:document onclick={outside} />

<div class="sel" bind:this={root} data-testid={testid}>
  <button class="trigger" onclick={() => (open = !open)} {onkeydown} aria-haspopup="listbox" aria-expanded={open}>
    {current?.label ?? placeholder}
    <span class="chev">▾</span>
  </button>
  {#if open}
    <ul role="listbox">
      {#each options as o, i (o.value)}
        <li role="option" aria-selected={o.value === value}>
          <button class:hi={i === hi} onclick={() => choose(o)} onmouseenter={() => (hi = i)}>{o.label}</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .sel { position: relative; }
  .trigger {
    width: 100%; display: flex; justify-content: space-between; align-items: center;
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 8px; font-size: var(--fs-1); cursor: pointer;
  }
  .trigger:focus { outline: none; border-color: var(--accent-dim); }
  .chev { color: var(--text-2); }
  ul {
    position: absolute; top: calc(100% + 2px); left: 0; right: 0; z-index: var(--layer-chrome);
    list-style: none; margin: 0; padding: var(--sp-1);
    background: var(--surface-2); border: 1px solid var(--border-0); border-radius: var(--r-2);
    box-shadow: 0 8px 24px oklch(0% 0 0 / 0.4); max-height: 220px; overflow-y: auto;
  }
  li button {
    width: 100%; text-align: left; background: none; border: 0; color: var(--text-0);
    padding: 6px 8px; border-radius: var(--r-1); cursor: pointer; font-size: var(--fs-1);
  }
  li button.hi { background: var(--surface-3); }
</style>
