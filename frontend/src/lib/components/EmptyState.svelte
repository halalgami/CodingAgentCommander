<script></script>

<div class="empty" data-testid="empty-state">
  <svg viewBox="0 0 64 64" width="72" height="72" aria-hidden="true">
    <path d="M32 8 L56 52 H8 Z" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linejoin="round" />
    <path d="M32 24 L45 48 H19 Z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" opacity="0.5" />
  </svg>
  <p class="title">No session on deck</p>
  <p class="hint"><kbd>⌘K</kbd> commands · <span>Launch</span> a session to begin</p>
</div>

<style>
  .empty {
    height: 100%; display: flex; flex-direction: column;
    align-items: center; justify-content: center; gap: var(--sp-3);
    color: var(--accent); background: var(--surface-0);
  }
  .title { color: var(--text-1); font-size: var(--fs-3); margin: 0; }
  .hint { color: var(--text-2); font-size: var(--fs-1); margin: 0; }
  .hint span { color: var(--text-1); }
  kbd {
    font-family: var(--font-mono); font-size: var(--fs-0);
    background: var(--surface-2); border: 1px solid var(--border-0);
    border-radius: var(--r-1); padding: 1px 5px;
  }
</style>
