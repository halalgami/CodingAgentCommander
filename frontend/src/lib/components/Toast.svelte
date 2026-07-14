<script>
  import { toasts } from "../stores.svelte.js";
</script>

<div class="toasts" data-testid="toasts">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}" role="status">{t.msg}</div>
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
  @keyframes in { from { transform: translateY(6px); opacity: 0; } }
</style>
