<script>
  import { app, confirmLaunch, cancelLaunch } from "../stores.svelte.js";
  import Select from "./Select.svelte";

  const lc = $derived(app.launchConfirm);

  let model = $state("");
  let launchBtn = $state(null);

  // When the modal opens, default the model to the entry's last-used one if it's
  // still in the catalog, else the current selection / first model; focus Launch.
  $effect(() => {
    if (lc) {
      const has = app.models.some((m) => m.id === lc.modelID);
      model = has ? lc.modelID : (app.selectedModel || app.models[0]?.id || "");
      queueMicrotask(() => launchBtn?.focus());
    }
  });

  function onkeydown(e) {
    // Let the model dropdown own its own Enter/Esc — choosing or opening a model
    // must not bubble up and trigger Launch/Cancel (that reintroduces exactly the
    // accidental-launch this modal exists to prevent).
    if (e.target.closest?.(".sel")) return;
    if (e.key === "Escape") { e.stopPropagation(); cancelLaunch(); }
    else if (e.key === "Enter") { e.preventDefault(); if (!lc.missing && model) confirmLaunch(lc.folder, model); }
  }
</script>

{#if lc}
  <div class="backdrop" onclick={cancelLaunch} aria-hidden="true"></div>
  <div class="modal" data-testid="launch-confirm" role="dialog" aria-modal="true"
       aria-label="Confirm launch" {onkeydown}>
    <h2>Launch session</h2>
    <p class="path" data-testid="launch-confirm-path">{lc.folder}</p>
    {#if lc.missing}
      <p class="warn" data-testid="launch-confirm-missing">This folder no longer exists on disk.</p>
    {/if}
    <label class="lbl">Model</label>
    <Select
      testid="launch-confirm-model"
      options={app.models.map((m) => ({ value: m.id, label: m.label + (m.routed && !m.ready ? " (needs key)" : "") }))}
      bind:value={model}
      placeholder="model…"
    />
    <div class="actions">
      <button class="ghost" data-testid="launch-confirm-cancel" onclick={cancelLaunch}>Cancel</button>
      <button class="go" data-testid="launch-confirm-go" bind:this={launchBtn}
              disabled={lc.missing || !model}
              onclick={() => confirmLaunch(lc.folder, model)}>Launch</button>
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: oklch(0% 0 0 / 0.45); z-index: var(--layer-backdrop); }
  .modal {
    position: fixed; top: 22%; left: 50%; transform: translateX(-50%);
    width: min(520px, 90vw); z-index: var(--layer-palette);
    background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-3);
    padding: var(--sp-4); box-shadow: 0 16px 48px oklch(0% 0 0 / 0.5);
    display: flex; flex-direction: column; gap: var(--sp-2);
    animation: pop var(--t-med) var(--ease-out);
  }
  @keyframes pop { from { transform: translateX(-50%) translateY(-6px); opacity: 0; } }
  h2 { margin: 0; font-size: var(--fs-2); font-weight: 600; letter-spacing: 0.04em; }
  .path {
    font-family: var(--font-mono); font-size: var(--fs-1); color: var(--text-1);
    margin: 0; word-break: break-all;
    background: var(--surface-2); border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 6px 8px;
  }
  .warn { color: var(--crit); font-size: var(--fs-1); margin: 0; }
  .lbl { font-size: var(--fs-0); color: var(--text-2); text-transform: uppercase; letter-spacing: 0.08em; }
  .actions { display: flex; justify-content: flex-end; gap: var(--sp-2); margin-top: var(--sp-2); }
  .ghost {
    background: var(--surface-2); color: var(--text-1); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 14px; cursor: pointer; font-size: var(--fs-1);
  }
  .ghost:hover { color: var(--text-0); background: var(--surface-3); }
  .go {
    background: var(--accent); color: var(--on-accent); border: 0;
    border-radius: var(--r-2); padding: 6px 18px; cursor: pointer;
    font-weight: 600; letter-spacing: 0.06em; font-size: var(--fs-1);
  }
  .go:hover:not(:disabled) { background: var(--accent-hover); }
  .go:disabled { background: var(--accent-dim); cursor: default; }
</style>
