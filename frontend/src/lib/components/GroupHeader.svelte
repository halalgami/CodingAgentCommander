<script>
  import GroupCaret from "./GroupCaret.svelte";

  // section: { key, group, count, attn } from deriveSections.
  let { section, ontoggle, onrename, ondelete, onremember, oncollapseothers, onkeymove, onpointerdown } = $props();
  const g = $derived(section.group);

  let renaming = $state(false);
  let val = $state("");
  let menuOpen = $state(false);

  function startRename() { menuOpen = false; renaming = true; val = g.name; }
  function commit() {
    if (renaming && val.trim()) onrename(g.id, val.trim());
    renaming = false;
  }
  function onkey(e) {
    if (e.altKey && e.shiftKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      onkeymove(g.id, e.key === "ArrowUp" ? -1 : 1);
    }
  }
  $effect(() => {
    if (!menuOpen) return;
    const k = (e) => { if (e.key === "Escape") { e.stopPropagation(); menuOpen = false; } };
    const d = (e) => { if (!e.target.closest || !e.target.closest(".gmenu-wrap")) menuOpen = false; };
    window.addEventListener("keydown", k, true);
    window.addEventListener("pointerdown", d, true);
    return () => { window.removeEventListener("keydown", k, true); window.removeEventListener("pointerdown", d, true); };
  });
</script>

<div class="ghead" data-testid="group-header" data-group-id={g.id} data-drop-key={g.id} data-drop-kind="header">
  {#if renaming}
    <!-- svelte-ignore a11y_autofocus -->
    <input class="grename" data-testid="group-rename" bind:value={val} autofocus
      onkeydown={(e) => { if (e.key === "Enter") commit(); if (e.key === "Escape") renaming = false; }}
      onblur={commit} />
  {:else}
    <button class="gtoggle" data-testid="group-toggle" aria-expanded={!g.collapsed}
      onclick={() => ontoggle(g.id, !g.collapsed)} onkeydown={onkey} onpointerdown={(e) => onpointerdown?.(e, g.id)}
      ondblclick={startRename}>
      <GroupCaret collapsed={g.collapsed} />
      <span class="gname">{g.name}</span>
      <span class="gcount">({section.count})</span>
      {#if section.attn}
        <span class="gattn" data-testid="group-attn" data-attn={section.attn} title={section.attn === "permission" ? "A session here needs your approval" : section.attn}></span>
      {/if}
    </button>
  {/if}
  <span class="gmenu-wrap">
    <button class="icon" data-testid="group-menu" aria-label="Group options" aria-haspopup="menu" aria-expanded={menuOpen}
      onclick={() => (menuOpen = !menuOpen)}>⋯</button>
    {#if menuOpen}
      <span class="menu" role="menu">
        <button role="menuitem" onclick={startRename}>Rename</button>
        <button role="menuitem" onclick={() => { menuOpen = false; oncollapseothers(g.id); }}>Collapse all others</button>
        <button role="menuitemcheckbox" aria-checked={!!g.remember} data-testid="group-remember"
          onclick={() => { menuOpen = false; onremember(g.id, !g.remember); }}>{g.remember ? "☑" : "☐"} New sessions in this folder join this group</button>
        <button role="menuitem" data-testid="group-delete" onclick={() => { menuOpen = false; ondelete(g.id); }}>Delete group</button>
      </span>
    {/if}
  </span>
</div>

<style>
  .ghead { display: flex; align-items: center; gap: 4px; margin: var(--sp-2) 0 2px; }
  .gtoggle {
    flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; background: none; border: 0;
    color: var(--text-1); font-size: var(--fs-1); padding: 3px 2px; cursor: pointer; text-align: left;
    touch-action: none;
  }
  .gtoggle:hover, .gtoggle:focus-visible { color: var(--text-0); }
  .gname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
  .gcount { color: var(--text-2); }
  .gattn { width: 8px; height: 8px; border-radius: 50%; flex: none; background: var(--text-2); }
  .gattn[data-attn="permission"] { background: var(--warn); box-shadow: 0 0 6px var(--warn); }
  .gattn[data-attn="error"] { background: var(--crit); }
  .gattn[data-attn="finished"] { background: var(--accent); }
  .grename { flex: 1; font-size: var(--fs-1); }
  .gmenu-wrap { position: relative; }
  .icon { background: none; border: 0; color: var(--text-2); cursor: pointer; padding: 2px 6px; border-radius: var(--r-1); }
  .icon:hover, .icon:focus-visible { color: var(--text-0); background: var(--surface-2); }
  .menu {
    position: absolute; right: 0; top: 100%; z-index: 20; display: flex; flex-direction: column; min-width: 240px;
    background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: 4px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }
  .menu button { background: none; border: 0; color: var(--text-0); text-align: left; padding: 6px 8px; border-radius: var(--r-1); cursor: pointer; font-size: var(--fs-1); }
  .menu button:hover, .menu button:focus-visible { background: var(--accent-faint); }
</style>
