<script>
  import {
    app, select, killSession, renameSession, swapSession, enableRemoteControl, ERROR_TTL_MS,
  } from "../stores.svelte.js";
  import { prefs, setPref } from "../prefs.svelte.js";
  import LaunchPanel from "./LaunchPanel.svelte";
  import SessionCard from "./SessionCard.svelte";

  // An optional snippet rendered in the band beneath the session list. The
  // sidebar owns the layout; it deliberately knows nothing about what goes in
  // here, which is what keeps this file free of any feature-specific import.
  let { dock } = $props();

  // The band's height is user-set because it CROPS its content: a flexible
  // height re-crops on every window resize, so the same art sits differently in
  // a small window than a large one. The sidebar still knows nothing about what
  // the band contains — only that its height is worth persisting.
  let aside = $state(null);
  let dockEl = $state(null);
  let dragging = $state(false);

  // Measure the band, never infer it. `flex: 1 0 40%` GROWS past its basis, so
  // computing a height from the basis reported a value well under the real one
  // — and the first arrow-key press then jumped the band smaller instead of
  // nudging it larger.
  const dockHeightNow = () => dockEl?.getBoundingClientRect().height ?? 0;

  function startResize(e) {
    e.preventDefault();
    dragging = true;
    const move = (ev) => {
      if (!aside || !dockEl) return;
      // Measured from the BAND's own bottom edge, not the column's: the aside
      // has padding, so using the column bottom offsets every drag by it.
      const bottom = dockEl.getBoundingClientRect().bottom;
      setPref("dockH", clampDock(bottom - ev.clientY, aside.clientHeight));
    };
    const up = () => {
      dragging = false;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }

  // Both ends are floors, not taste: below MIN the art is unreadable, and above
  // the cap the session list loses the room it needs to stay browsable — which
  // was the point of putting the band under the list in the first place.
  const DOCK_MIN = 140;
  function clampDock(px, columnPx) {
    const max = Math.max(DOCK_MIN, Math.round(columnPx * 0.7));
    return Math.round(Math.min(max, Math.max(DOCK_MIN, px)));
  }

  // Reported to assistive tech, and the same numbers the drag clamps to.
  const dockMax = $derived(Math.max(DOCK_MIN, Math.round((aside?.clientHeight ?? 800) * 0.7)));
  const dockPx = $derived(prefs.dockH > 0 ? prefs.dockH : Math.round(dockHeightNow()));

  // Keyboard parity: a pointer-only resize is unreachable for anyone who cannot
  // drag, and this one persists real state.
  function onHandleKey(e) {
    const step = e.shiftKey ? 40 : 12;
    let delta = 0;
    if (e.key === "ArrowUp") delta = step;
    else if (e.key === "ArrowDown") delta = -step;
    else return;
    e.preventDefault();
    const current = prefs.dockH || dockHeightNow();
    setPref("dockH", clampDock(current + delta, aside?.clientHeight ?? 800));
  }

  // The latch (app.errorMs) attributes to whichever session is selected when
  // it fires — see stores.svelte.js's ERROR_TTL_MS comment — so only the
  // active card can ever be errored, mirroring isActive's own computation.
  function isErrored(windowID) {
    return app.errorMs > 0 && Date.now() - app.errorMs < ERROR_TTL_MS &&
      app.sessionKey.split(":")[0] === windowID;
  }
</script>

<aside data-testid="sidebar" bind:this={aside}>
  <LaunchPanel />
  <ul class="sessions" data-testid="session-list">
    {#each app.sessions as s (s.windowID)}
      <SessionCard
        session={s}
        stat={app.stats[s.windowID]}
        isActive={app.sessionKey.split(":")[0] === s.windowID}
        isFinished={!!app.finished[s.windowID]}
        isErrored={isErrored(s.windowID)}
        models={app.models}
        onselect={select}
        onrename={renameSession}
        onkill={killSession}
        onswap={swapSession}
        onrc={enableRemoteControl}
      />
    {/each}
  </ul>
  {#if dock}
    <!-- The WAI-ARIA "Window Splitter" pattern: a FOCUSABLE separator carrying
         its own value, adjusted with the arrow keys. svelte-ignore is warranted
         because the rule assumes every separator is decorative, which is the
         case this pattern is the documented exception to. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="dock-handle" data-testid="sidebar-dock-handle"
      class:dragging
      role="separator" aria-orientation="horizontal" aria-label="Resize the panel below the session list"
      aria-valuenow={dockPx} aria-valuemin={DOCK_MIN} aria-valuemax={dockMax}
      tabindex="0" onpointerdown={startResize} onkeydown={onHandleKey}
    ></div>
  {/if}
  <div
    class="dock" data-testid="sidebar-dock" bind:this={dockEl}
    style:flex-basis={prefs.dockH > 0 ? prefs.dockH + "px" : null}
    style:flex-grow={prefs.dockH > 0 ? "0" : null}
  >{@render dock?.()}</div>
</aside>

<style>
  aside {
    width: var(--sidebar-w); flex: none; height: 100%;
    background: var(--surface-1); border-right: 1px solid var(--border-0);
    padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3);
    /* The aside must CLIP, not scroll. It used to be `overflow-y: auto`, which
       made the whole column one scroller — with a dock at the bottom that means
       the dock scrolls out of view instead of staying put. */
    overflow: hidden;
    min-height: 0;
  }
  /* flex: 0 1 auto — grow no further than the content, shrink freely.
     min-height: 0 is REQUIRED: the default `min-height: auto` on a flex item
     refuses to shrink below its content, so the list would push the dock off the
     bottom and its own scrollbar would never appear. */
  .sessions {
    flex: 0 1 auto; min-height: 0; overflow-y: auto;
    margin: 0; padding: 0;
  }
  /* flex: 1 0 40% — take the remainder, and never shrink below 40% of the
     sidebar height however many sessions exist. That is the cap §4.1 asks for,
     expressed as the dock's floor rather than the list's ceiling, so a sidebar
     with three sessions still gives the list all the room it wants. */
  .dock { flex: 1 0 40%; min-height: 0; display: flex; }
  /* No snippet passed (the public build passes none) -> no band at all, and the
     list gets the whole column exactly as it did before. */
  .dock:empty { display: none; }
  /* Never taller than 70% of the column, whatever is stored: a persisted height
     from a large window must not squeeze the session list to nothing when the
     same prefs are read back in a small one. */
  .dock { max-height: 70%; }

  .dock-handle {
    flex: none; height: 8px; margin: calc(var(--sp-2) * -1) 0 calc(var(--sp-2) * -1);
    cursor: row-resize; background: transparent; border-radius: var(--r-1);
    transition: background var(--t-fast) var(--ease-out);
  }
  .dock-handle:hover, .dock-handle.dragging, .dock-handle:focus-visible {
    background: var(--accent-dim);
  }
  .dock-handle:focus-visible { outline: 1px solid var(--accent); outline-offset: 1px; }
  /* No band, no handle: the {#if dock} guard above is what enforces this — the
     public build passes no dock snippet, so neither element is rendered. */
</style>
