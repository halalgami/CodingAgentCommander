<script>
  let { title, testid, onclose, wide = false, children } = $props();
  let root = $state(null);

  // Focus scoping per spec: ESC lives on the drawer element, never window;
  // the terminal keeps its interrupt key when no overlay is focused.
  $effect(() => {
    if (!root) return;
    const prev = document.activeElement;
    const first = root.querySelector("input, button, select, [tabindex]");
    (first ?? root).focus();
    return () => prev?.focus?.();
  });

  function onkeydown(e) {
    if (e.key === "Escape") { e.stopPropagation(); onclose(); return; }
    if (e.key !== "Tab") return;
    const els = [...root.querySelectorAll("input, button, select, [tabindex]")].filter((n) => !n.disabled);
    if (!els.length) return;
    const first = els[0], last = els[els.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }

  // A reactive update (e.g. a button toggling to a different branch) can destroy
  // the focused element; the browser then parks focus on <body>, which the
  // element-scoped handler above never sees, and Escape stops closing the drawer.
  //
  // This listens at the window instead of pulling focus back. Reclaiming focus
  // was tried and is WRONG: a real mouse click blurs the old element before it
  // focuses the new one, so a reclaim fires in that gap and steals focus from
  // the input being clicked — every drawer input becomes untypable, and no
  // fill()-based test catches it because fill() doesn't blur first.
  //
  // Scoped by the same signal, read rather than written: act only when focus
  // sits on <body>. If anything else holds focus — the palette, a modal,
  // another overlay — that thing owns Escape.
  $effect(() => {
    function onWindowKey(e) {
      if (e.key !== "Escape") return;
      if (document.activeElement && document.activeElement !== document.body) return;
      e.preventDefault();
      onclose();
    }
    window.addEventListener("keydown", onWindowKey, true);
    return () => window.removeEventListener("keydown", onWindowKey, true);
  });
</script>

<div class="backdrop" onclick={onclose} aria-hidden="true"></div>
<div
  class="drawer" class:wide={wide} data-testid={testid} bind:this={root} {onkeydown}
  role="dialog" aria-modal="true" aria-label={title} tabindex="-1"
>
  <header>
    <h2>{title}</h2>
    <button class="close" data-testid="{testid}-close" onclick={onclose} title="Close">✕</button>
  </header>
  <div class="body">
    {@render children()}
  </div>
</div>

<style>
  .backdrop {
    position: fixed; inset: 0; background: oklch(0% 0 0 / 0.45);
    z-index: var(--layer-backdrop);
  }
  .drawer {
    position: fixed; top: 0; right: 0; bottom: 0; width: 380px;
    background: var(--surface-1); border-left: 1px solid var(--border-0);
    z-index: var(--layer-drawer); display: flex; flex-direction: column;
    animation: slide var(--t-med) var(--ease-out);
    outline: none;
  }
  /* Opt-in only. The default 380px is what every other drawer relies on; a
     document full of tables is the one thing that needs more. */
  .drawer.wide { width: min(720px, 90vw); }
  @keyframes slide { from { transform: translateX(24px); opacity: 0; } }
  header {
    display: flex; align-items: center; justify-content: space-between;
    padding: var(--sp-3) var(--sp-4); border-bottom: 1px solid var(--border-0);
  }
  h2 { margin: 0; font-size: var(--fs-3); font-weight: 600; letter-spacing: 0.06em; }
  .close { background: none; border: 0; color: var(--text-1); cursor: pointer; font-size: var(--fs-2); }
  .close:hover { color: var(--text-0); }
  .body { padding: var(--sp-4); overflow-y: auto; flex: 1; }
</style>
