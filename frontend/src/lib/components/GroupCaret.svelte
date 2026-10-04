<script>
  let { collapsed } = $props();
</script>

<span class="gcaret" aria-hidden="true" data-state={collapsed ? "collapsed" : "expanded"}>
  <svg viewBox="0 0 12 12" width="12" height="12" fill="none" stroke="currentColor"
       stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M4.5 2.5 8 6l-3.5 3.5" />
  </svg>
</span>

<style>
  .gcaret {
    flex: none; display: inline-flex; align-items: center; justify-content: center;
    width: 18px; height: 18px; border-radius: 4px;
    background: var(--surface-3); border: 1px solid var(--border-0);
    color: var(--text-2);
  }
  .gcaret[data-state="expanded"] { color: var(--text-0); border-color: var(--border-1); }
  svg { transition: transform 120ms ease; }
  .gcaret[data-state="expanded"] svg { transform: rotate(90deg); }
  @media (prefers-reduced-motion: reduce) { svg { transition: none; } }
</style>
