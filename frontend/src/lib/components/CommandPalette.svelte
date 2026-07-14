<script>
  import { app, select, launchInto, swapSession, enableRemoteControl } from "../stores.svelte.js";
  import { loadRecents } from "../recents.js";
  import { fuzzyFilter } from "../fuzzy.js";

  let query = $state("");
  let hi = $state(0);
  let inputEl = $state(null);

  // Window-level CAPTURE listener so ⌘K works while xterm has focus.
  // Matches ⌘K exclusively — ESC stays element-scoped per spec.
  function globalKey(e) {
    if (e.metaKey && e.key.toLowerCase() === "k") {
      e.preventDefault();
      e.stopPropagation();
      app.paletteOpen = !app.paletteOpen;
      query = ""; hi = 0;
    }
  }
  $effect(() => {
    window.addEventListener("keydown", globalKey, true);
    return () => window.removeEventListener("keydown", globalKey, true);
  });
  $effect(() => {
    if (app.paletteOpen && inputEl) inputEl.focus();
  });

  const actions = $derived.by(() => {
    const items = [];
    for (const s of app.sessions) {
      items.push({ label: `Go: ${s.name}`, hint: "session", run: () => select(s.windowID) });
    }
    for (const r of loadRecents()) {
      const short = r.folder.split("/").slice(-2).join("/");
      items.push({ label: `Launch: ${short}`, hint: r.modelID, run: () => launchInto(r.folder, r.modelID) });
    }
    if (app.sessionKey) {
      const wid = app.sessionKey.split(":")[0];
      for (const m of app.models) {
        items.push({ label: `Swap to: ${m.label}`, hint: "model", run: () => swapSession(wid, m.id) });
      }
    }
    if (app.sessionKey) {
      const wid = app.sessionKey.split(":")[0];
      const st = app.stats[wid];
      if (st && st.provider === "anthropic" && !st.remoteControl) {
        items.push({ label: "Hand off to phone", hint: "remote", run: () => enableRemoteControl(wid) });
      }
    }
    items.push({ label: "Settings", hint: "config", run: () => (app.drawer = "settings") });
    items.push({ label: "Providers", hint: "config", run: () => (app.drawer = "providers") });
    items.push({ label: "Models", hint: "config", run: () => (app.drawer = "models") });
    return items;
  });

  const results = $derived(fuzzyFilter(query, actions, (a) => a.label).slice(0, 12));

  function pick(a) {
    app.paletteOpen = false;
    a.run();
  }
  function onkeydown(e) {
    if (e.key === "Escape") { e.stopPropagation(); app.paletteOpen = false; }
    if (e.key === "ArrowDown") { e.preventDefault(); hi = Math.min(hi + 1, results.length - 1); }
    if (e.key === "ArrowUp") { e.preventDefault(); hi = Math.max(hi - 1, 0); }
    if (e.key === "Enter" && results[hi]) { e.preventDefault(); pick(results[hi]); }
  }
</script>

{#if app.paletteOpen}
  <div class="backdrop" onclick={() => (app.paletteOpen = false)} aria-hidden="true"></div>
  <div class="palette" data-testid="palette" role="dialog" aria-modal="true" aria-label="Commands">
    <input
      bind:this={inputEl} bind:value={query} {onkeydown}
      data-testid="palette-input" placeholder="Jump, launch, swap, configure…"
      oninput={() => (hi = 0)}
    />
    <ul>
      {#each results as a, i (a.label + ":" + i)}
        <li>
          <button class:hi={i === hi} onclick={() => pick(a)} onmouseenter={() => (hi = i)}>
            <span>{a.label}</span><span class="hint mono">{a.hint}</span>
          </button>
        </li>
      {/each}
      {#if !results.length}<li class="none">No matches</li>{/if}
    </ul>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: oklch(0% 0 0 / 0.45); z-index: var(--layer-backdrop); }
  .palette {
    position: fixed; top: 18%; left: 50%; transform: translateX(-50%);
    width: min(560px, 90vw); z-index: var(--layer-palette);
    background: var(--surface-1); border: 1px solid var(--border-1);
    border-radius: var(--r-3); overflow: hidden;
    box-shadow: 0 16px 48px oklch(0% 0 0 / 0.5);
    animation: pop var(--t-med) var(--ease-out);
  }
  @keyframes pop { from { transform: translateX(-50%) translateY(-6px); opacity: 0; } }
  input {
    width: 100%; background: none; border: 0; border-bottom: 1px solid var(--border-0);
    color: var(--text-0); padding: var(--sp-3) var(--sp-4); font-size: var(--fs-3);
  }
  input:focus { outline: none; }
  ul { list-style: none; margin: 0; padding: var(--sp-1); max-height: 320px; overflow-y: auto; }
  li button {
    width: 100%; display: flex; justify-content: space-between; align-items: center;
    background: none; border: 0; color: var(--text-0); cursor: pointer;
    padding: var(--sp-2) var(--sp-3); border-radius: var(--r-1); font-size: var(--fs-2);
  }
  li button.hi { background: var(--accent-faint); }
  .hint { color: var(--text-2); font-size: var(--fs-0); }
  .mono { font-family: var(--font-mono); }
  .none { color: var(--text-2); padding: var(--sp-3); font-size: var(--fs-1); }
</style>
