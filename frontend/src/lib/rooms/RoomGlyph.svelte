<script>
  // The session card's room icon (UX spec §3). Carries state, so it is
  // always visible once the project is shared; unshared it shows on hover.
  import { rooms, projectOf, summaryOf, openPanelFor } from "./rooms.svelte.js";
  import { glyphState } from "./rooms.js";
  import SharePopover from "./SharePopover.svelte";
  let { cwd, windowID } = $props();

  const pr = $derived(projectOf(cwd));
  const g = $derived(glyphState(pr, pr?.roomId ? summaryOf(pr.roomId) : undefined));
  const sharing = $derived(rooms.share?.windowID === windowID);

  function click(e) {
    e.stopPropagation();
    if (g.kind === "none") {
      rooms.error = "";
      rooms.share = sharing ? null : { windowID, cwd, label: pr?.label || cwd.split(/[\\/]/).filter(Boolean).pop() || "this project" };
    } else openPanelFor(pr.roomId);
  }
</script>

{#if cwd}
  <span class="wrap">
    <button class="rglyph" data-testid="room-glyph" data-kind={g.kind} aria-label={g.label} title={g.label}
      aria-haspopup={g.kind === "none" ? "dialog" : undefined} aria-expanded={g.kind === "none" ? sharing : undefined} onclick={click}>
      <span aria-hidden="true">👥</span>
      {#if g.unread}<span class="badge" data-testid="room-glyph-unread">{g.unread}</span>{/if}
      {#if g.kind === "needs-folder" || g.queued}<span class="dot amber" aria-hidden="true"></span>
      {:else if g.online}<span class="dot green" aria-hidden="true"></span>{/if}
    </button>
    {#if sharing}<SharePopover cwd={rooms.share.cwd} label={rooms.share.label} />{/if}
  </span>
{/if}

<style>
  .wrap { position: relative; flex: none; display: inline-flex; }
  .rglyph {
    position: relative; background: none; border: 0; cursor: pointer; padding: 2px 4px; border-radius: var(--r-1);
    font-size: var(--fs-1); line-height: 1; color: var(--text-1);
  }
  .rglyph:hover, .rglyph:focus-visible { background: var(--surface-3); }
  .rglyph[data-kind="none"] { opacity: 0.55; filter: grayscale(1); }
  .badge {
    position: absolute; top: -4px; right: -6px; min-width: 14px; padding: 0 3px; border-radius: 7px;
    background: var(--accent, #3987e5); color: #fff; font-size: 9px; line-height: 14px; text-align: center;
  }
  .dot { position: absolute; bottom: 0; right: 0; width: 6px; height: 6px; border-radius: 50%; }
  .dot.green { background: var(--ok, #2fa84f); }
  .dot.amber { background: #c98500; }
</style>
