<script>
  import { app, launch, launchInto, pickFolder } from "../stores.svelte.js";
  import { loadRecents } from "../recents.js";
  import { prefs } from "../prefs.svelte.js";
  import Select from "./Select.svelte";

  let recentsOpen = $state(false);
  let recents = $state([]);
  let busy = $state(false);

  const selectedRouted = $derived(!!app.models.find((m) => m.id === app.selectedModel)?.routed);
  const rcWanted = $derived(prefs.rcAutoEnable);

  function toggleRecents() {
    recents = loadRecents();
    recentsOpen = !recentsOpen;
  }
  async function go() {
    busy = true;
    await launch();
    busy = false;
    recentsOpen = false;
  }
  async function goRecent(r) {
    recentsOpen = false;
    busy = true;
    await launchInto(r.folder, r.modelID);
    busy = false;
  }
  function label(id) {
    return app.models.find((m) => m.id === id)?.label ?? id;
  }
</script>

<div class="panel">
  <div class="folder">
    <input
      data-testid="folder-input"
      placeholder="/path/to/project"
      bind:value={app.folder}
      onfocus={() => { recents = loadRecents(); recentsOpen = recents.length > 0; }}
      onblur={() => setTimeout(() => (recentsOpen = false), 150)}
    />
    <button class="ghost" data-testid="pick-folder" title="Choose folder" onclick={pickFolder}>▸</button>
    <button class="ghost" data-testid="toggle-recents" title="Recent launches" onclick={toggleRecents}>⏱</button>
  </div>
  {#if recentsOpen && recents.length}
    <ul class="recents" data-testid="recents-list">
      {#each recents as r (r.folder + r.modelID)}
        <li>
          <button onclick={() => goRecent(r)}>
            <span class="path">{r.folder.split("/").slice(-2).join("/")}</span>
            <span class="model">{label(r.modelID)}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <Select
    testid="model-select"
    options={app.models.map((m) => ({ value: m.id, label: m.label + (m.routed && !m.ready ? " (needs key)" : "") }))}
    bind:value={app.selectedModel}
    placeholder="model…"
  />
  {#if rcWanted && selectedRouted}
    <p class="dim" data-testid="rc-hint">Remote control skipped — native Anthropic sessions only</p>
  {/if}
  <button class="launch" data-testid="launch-button" disabled={busy} onclick={go}>
    {busy ? "LAUNCHING…" : "LAUNCH"}
  </button>
  {#if app.launchError}
    <p class="err" data-testid="launch-error">{app.launchError}</p>
  {/if}
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-2); position: relative; }
  .folder { display: flex; gap: var(--sp-1); }
  input {
    background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 6px 8px; font-size: var(--fs-1); min-width: 0;
    flex: 1; font-family: var(--font-mono);
  }
  input:focus { outline: none; border-color: var(--accent-dim); }
  .ghost {
    background: var(--surface-2); color: var(--text-1); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 0 9px; cursor: pointer;
  }
  .ghost:hover { color: var(--text-0); background: var(--surface-3); }
  .recents {
    position: absolute; top: 34px; left: 0; right: 0; z-index: var(--layer-chrome);
    list-style: none; margin: 0; padding: var(--sp-1);
    background: var(--surface-2); border: 1px solid var(--border-0); border-radius: var(--r-2);
    box-shadow: 0 8px 24px oklch(0% 0 0 / 0.4);
  }
  .recents button {
    display: flex; justify-content: space-between; gap: var(--sp-2); width: 100%;
    background: none; border: 0; color: var(--text-0); padding: 6px 8px;
    border-radius: var(--r-1); cursor: pointer; font-size: var(--fs-1);
  }
  .recents button:hover { background: var(--surface-3); }
  .path { font-family: var(--font-mono); }
  .model { color: var(--text-1); }
  .launch {
    background: var(--accent); color: var(--on-accent); border: 0;
    border-radius: var(--r-2); padding: 8px; cursor: pointer;
    font-weight: 600; letter-spacing: 0.08em; font-size: var(--fs-1);
    transition: background var(--t-fast) var(--ease-out), transform var(--t-fast) var(--ease-out);
  }
  .launch:hover:not(:disabled) { background: var(--accent-hover); }
  .launch:active:not(:disabled) { transform: scale(0.98); }
  .launch:disabled { background: var(--accent-dim); cursor: default; }
  .err { color: var(--crit); font-size: var(--fs-1); margin: 0; }
  .dim { color: var(--text-2); font-size: var(--fs-0); margin: 0; }
</style>
