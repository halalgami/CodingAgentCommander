<script>
  import { toasts } from "../stores.svelte.js";
  import AttachConfirm from "./AttachConfirm.svelte";
</script>

<div class="toasts" data-testid="toasts">
  <AttachConfirm />
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}" role="status" data-testid="toast">{t.msg}{#if t.action}
      <button class="act" data-testid="toast-action" onclick={() => { t.action.run(); const i = toasts.findIndex((x) => x.id === t.id); if (i !== -1) toasts.splice(i, 1); }}>{t.action.label}</button>{/if}</div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed; bottom: var(--sp-4); right: var(--sp-4);
    display: flex; flex-direction: column; gap: var(--sp-2);
    z-index: var(--layer-toast); pointer-events: none;
  }
  .toast {
    background: var(--surface-2); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: var(--sp-2) var(--sp-3);
    font-size: var(--fs-1); max-width: 340px;
    animation: in var(--t-med) var(--ease-out);
    box-shadow: 0 8px 24px oklch(0% 0 0 / 0.4);
  }
  .toast.error { border-color: var(--crit); }
  .act { pointer-events: auto; margin-left: var(--sp-2); background: none; border: 1px solid var(--accent-dim); color: var(--accent); border-radius: var(--r-1); padding: 1px 8px; cursor: pointer; }
  @keyframes in { from { transform: translateY(6px); opacity: 0; } }
</style>
