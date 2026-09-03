<script>
  import { untrack } from "svelte";
  import { app, select, swapSession, enableRemoteControl, askLaunch } from "../stores.svelte.js";
  import { listProjects } from "../history.js";
  import { fuzzyFilter } from "../fuzzy.js";
  import { listProjectDocs } from "../projectdocs.js";
  import { openDoc } from "../docview.svelte.js";
  import { pickDocRoot, docPaletteItems } from "../docs.js";

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
  let histItems = $state([]);
  $effect(() => {
    if (app.paletteOpen) {
      listProjects().then((p) => { histItems = p; }).catch(() => { histItems = []; });
    }
  });

  let docRoot = $state("");
  let docList = $state({ entries: [], truncated: false });
  let docNow = $state(0);
  let lastListedRoot = "";
  $effect(() => {
    // Depend on exactly two things: the palette opening, and history arriving
    // (which supplies the fallback root). Everything else is read inside
    // untrack, because app.stats is reassigned by the 5s session poll —
    // tracking it would spawn a `git ls-files` every five seconds for as long
    // as the palette stayed open.
    const open = app.paletteOpen;
    const histCount = histItems.length;
    if (!open) { lastListedRoot = ""; return; }
    untrack(() => {
      void histCount;
      const wid = app.sessionKey ? app.sessionKey.split(":")[0] : "";
      const root = pickDocRoot(app.stats[wid]?.cwd ?? "", histItems);
      docRoot = root;
      if (!root || root === lastListedRoot) return;
      lastListedRoot = root;
      docNow = Math.floor(Date.now() / 1000);
      listProjectDocs(root)
        .then((l) => { docList = l; })
        .catch(() => { docList = { entries: [], truncated: false }; });
    });
  });

  // Reads `query` now (it didn't before): an empty query surfaces only the
  // 3 most-recent documents ahead of the config rows so ⌘K actually
  // advertises that documents exist, while a non-empty query still offers
  // every document, last, for fuzzy scoring to rank. That means this whole
  // array is rebuilt on every keystroke rather than once per palette open —
  // acceptable, since it is a small array (sessions + history + a handful of
  // config/doc rows), not a hot loop.
  const actions = $derived.by(() => {
    const hasQuery = !!query;
    const items = [];
    for (const s of app.sessions) {
      items.push({ label: `Go: ${s.name}`, hint: "session", run: () => select(s.windowID) });
    }
    for (const r of histItems) {
      const short = r.folder.split("/").slice(-2).join("/");
      items.push({ label: `Open: ${short}`, hint: r.lastModelID, run: () => askLaunch(r.folder, r.lastModelID, r.missing) });
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
    const docItems = docPaletteItems(docList, docNow).map((d) => ({
      label: d.label, hint: d.hint, run: () => openDoc(docRoot, d.rel),
    }));
    // Empty query: docs would otherwise never survive the top-12 render cap
    // behind enough sessions/history rows, so a few go in early — still after
    // every session/history/model row (those stay reachable first), but
    // ahead of the config rows below rather than behind all of them.
    if (!hasQuery) items.push(...docItems.slice(0, 3));
    items.push({ label: "Settings", hint: "config", run: () => (app.drawer = "settings") });
    items.push({ label: "Providers", hint: "config", run: () => (app.drawer = "providers") });
    items.push({ label: "Models", hint: "config", run: () => (app.drawer = "models") });
    items.push({ label: "About", hint: "app", run: () => (app.about = true) });
    // Non-empty query: every doc is offered, still last so it cannot evict a
    // command, and fuzzy scoring alone decides whether it survives the slice.
    if (hasQuery) items.push(...docItems);
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
      {#if docList.truncated}
        <li class="none" data-testid="palette-docs-truncated">
          Some documents are not listed — narrow the project or open it externally
        </li>
      {/if}
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
