<script>
  import GroupCaret from "./GroupCaret.svelte";
  let {
    options = [], groups = null, value = $bindable(""),
    placeholder = "select…", testid, onchange, ontogglegroup,
  } = $props();
  let open = $state(false);
  let root = $state(null);
  let hi = $state(0);

  // One flat render list, so markup order and keyboard order cannot drift
  // apart. Headers are rows too; `selectable` is the subset arrows walk.
  const rows = $derived.by(() => {
    if (!groups) return options.map((o) => ({ kind: "option", o }));
    const out = [];
    for (const g of groups) {
      if (!g.single) out.push({ kind: "header", g });
      if (g.collapsed) continue;
      for (const m of g.models) {
        out.push({ kind: "option", o: { value: m.id, label: m.displayLabel ?? m.label, full: m.label, suffix: m.suffix } });
      }
    }
    return out;
  });
  const selectable = $derived(rows.filter((r) => r.kind === "option").map((r) => r.o));
  // The trigger shows the full label (provider included): there is no header
  // beside it to supply that context. Looked up across ALL groups so it does
  // not depend on what is currently expanded.
  const currentLabel = $derived.by(() => {
    if (!groups) return options.find((o) => o.value === value)?.label;
    for (const g of groups) {
      const m = g.models.find((x) => x.id === value);
      if (m) return m.label + (m.suffix ?? "");
    }
    return undefined;
  });

  function choose(o) {
    value = o.value;
    open = false;
    onchange?.(o.value);
  }
  function onkeydown(e) {
    if (!open && (e.key === "Enter" || e.key === " " || e.key === "ArrowDown")) { e.preventDefault(); open = true; hi = 0; return; }
    if (!open) return;
    if (e.key === "Escape") { e.stopPropagation(); open = false; }
    if (e.key === "ArrowDown") { e.preventDefault(); hi = Math.min(hi + 1, selectable.length - 1); }
    if (e.key === "ArrowUp") { e.preventDefault(); hi = Math.max(hi - 1, 0); }
    if (e.key === "Enter") { e.preventDefault(); if (selectable[hi]) choose(selectable[hi]); }
  }
  function outside(e) {
    if (root && !root.contains(e.target)) open = false;
  }
  // Index among SELECTABLE rows, for highlight comparison.
  function optionIndex(rowIdx) {
    let n = 0;
    for (let i = 0; i < rowIdx; i++) if (rows[i].kind === "option") n++;
    return n;
  }
</script>

<svelte:document onclick={outside} />

<div class="sel" bind:this={root} data-testid={testid}>
  <button class="trigger" onclick={() => (open = !open)} {onkeydown} aria-haspopup="listbox" aria-expanded={open}>
    {currentLabel ?? placeholder}
    <span class="chev">▾</span>
  </button>
  {#if open}
    <ul role="listbox">
      {#each rows as r, i (r.kind === "header" ? "h:" + r.g.provider : "o:" + r.o.value)}
        {#if r.kind === "header"}
          <li role="presentation" class="grouphdr">
            <!-- mousedown default prevented so a click does not pull focus off the
                 trigger, which owns the only keydown handler. -->
            <button type="button" aria-expanded={!r.g.collapsed}
                    onmousedown={(e) => e.preventDefault()}
                    onclick={(e) => { e.stopPropagation(); ontogglegroup?.(r.g.provider); }}>
              <GroupCaret collapsed={r.g.collapsed} />
              <span class="name">{r.g.label}</span>
              <span class="count">{r.g.count}</span>
            </button>
          </li>
        {:else}
          <li role="option" aria-selected={r.o.value === value}>
            <button class:hi={optionIndex(i) === hi} onclick={() => choose(r.o)}
                    onmouseenter={() => (hi = optionIndex(i))}>{r.o.label}{r.o.suffix ?? ""}</button>
          </li>
        {/if}
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
  .grouphdr button {
    width: 100%; display: flex; align-items: center; gap: var(--sp-1);
    background: none; border: 0; cursor: pointer;
    color: var(--text-2); font-size: var(--fs-0); letter-spacing: 0.08em;
    text-transform: uppercase; padding: 6px 8px;
  }
  .grouphdr button:hover { background: var(--surface-3); }
  .grouphdr button:disabled { cursor: default; }
  .grouphdr .name { flex: 1; text-align: left; }
  .grouphdr .count { color: var(--text-2); font-variant-numeric: tabular-nums; }
  .grouphdr + li button { padding-left: var(--sp-3); }
</style>
