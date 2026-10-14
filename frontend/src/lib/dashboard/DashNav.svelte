<script>
  import { app } from "../stores.svelte.js";
  import { isHome, setMode, goHome, modes, MODE_GROUPS, currentMode } from "./dash.svelte.js";
  import Sigil from "./Sigil.svelte";
  const sid = $derived(app.stats[app.sessionKey.split(":")[0]]?.sessionID || app.sessionKey);
  const cur = $derived(modes.find((m) => m.id === currentMode()) || modes[0]);
  let open = $state(false);
  let btn = $state(), menu = $state();
  const items = () => [...(menu?.querySelectorAll('[role="menuitemradio"]') || [])];
  function toggle() {
    open = !open;
    if (open) queueMicrotask(() => items().find((el) => el.getAttribute("aria-checked") === "true")?.focus());
  }
  function close(refocus) { open = false; if (refocus) btn?.focus(); }
  // Focus goes to the dashboard, not back to the switcher, so a map's ←/→ keys work at once.
  function pick(id) {
    setMode(id); close(false);
    queueMicrotask(() => document.querySelector('[data-testid="dash-home"]')?.focus());
  }
  function onmenukey(e) {
    const list = items(), i = list.indexOf(document.activeElement);
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(true); }
    else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const d = e.key === "ArrowDown" ? 1 : -1;
      list[(i + d + list.length) % list.length]?.focus();
    } else if (e.key === "Home" || e.key === "End") {
      e.preventDefault();
      (e.key === "Home" ? list[0] : list[list.length - 1])?.focus();
    }
  }
  // Tabbing out of the menu closes it. A focusout with no target is WebKit
  // blurring the item on a click; the outside-pointerdown handler owns clicks.
  function onmenufocusout(e) {
    if (open && e.relatedTarget && !menu?.contains(e.relatedTarget) && !btn?.contains(e.relatedTarget)) close(false);
  }
  // the menu belongs to the home view: leaving home closes it, so it never comes back open
  $effect(() => { if (!isHome()) open = false; });
  $effect(() => {
    if (!open) return;
    const away = (e) => { if (!menu?.contains(e.target) && !btn?.contains(e.target)) close(false); };
    window.addEventListener("pointerdown", away, true);
    return () => window.removeEventListener("pointerdown", away, true);
  });
</script>

<div class="dashnav" data-testid="dash-nav">
  {#if isHome()}
    <button bind:this={btn} class="switch" data-testid="dash-mode-switch" aria-haspopup="menu" aria-expanded={open}
      title="Dashboard mode (⌘1–⌘{modes.length})" onclick={toggle}>{cur.label} <span class="caret">▾</span></button>
    <div bind:this={menu} class="menu" role="menu" tabindex="-1" hidden={!open} data-testid="dash-mode-menu" onkeydown={onmenukey} onfocusout={onmenufocusout}>
      {#each MODE_GROUPS as g (g)}
        <div role="group" aria-label={g}>
          <div class="hd" aria-hidden="true">{g.toUpperCase()}</div>
          {#each modes.filter((m) => m.group === g) as m (m.id)}
            <button role="menuitemradio" aria-checked={currentMode() === m.id} data-testid="dash-mode-{m.id}"
              class:on={currentMode() === m.id} onclick={() => pick(m.id)}><span>{m.label}</span><kbd>⌘{m.key}</kbd></button>
          {/each}
        </div>
      {/each}
    </div>
  {:else}
    <button data-testid="dash-back" title="Dashboard (⌘D)" onclick={goHome}>← Dashboard</button>
    <Sigil id={sid} size={16} />
  {/if}
</div>

<style>
  .dashnav { --wails-draggable: no-drag; position: relative; display: flex; align-items: center; gap: 2px; margin-left: var(--sp-4); }
  button {
    --wails-draggable: no-drag; background: none; border: 0; color: var(--text-1); cursor: pointer;
    font-size: var(--fs-1); padding: 4px 8px; border-radius: var(--r-1); white-space: nowrap;
  }
  button:hover { color: var(--text-0); background: var(--surface-2); }
  .switch { color: var(--text-0); background: var(--surface-3); }
  .caret { font-size: 0.8em; opacity: 0.7; }
  .menu { position: absolute; left: 0; top: calc(100% + 4px); z-index: 50; min-width: 200px; padding: 4px; display: grid;
    background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4); }
  .menu[hidden] { display: none; }
  .hd { font-size: 10px; letter-spacing: 0.08em; color: var(--text-2); padding: 6px 8px 2px; }
  .menu button { display: flex; justify-content: space-between; gap: 16px; text-align: left; }
  .menu button.on { color: var(--text-0); background: var(--accent, #3b5bdb); }
  .menu button:focus-visible { outline: 1px solid var(--text-1); }
  kbd { font-family: inherit; color: var(--text-2); }
  .menu button.on kbd { color: inherit; opacity: 0.8; }
</style>
