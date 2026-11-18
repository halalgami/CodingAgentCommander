<script>
  // "Share this project" (UX spec §5): two clicks from the card. Inline
  // setup the first time. Closes on Escape or a click outside.
  import { rooms, shareProject } from "./rooms.svelte.js";
  import SetupStep from "./SetupStep.svelte";
  let { cwd, label } = $props();
  let el = $state(null);

  const st = $derived(rooms.status);
  const needsSetup = $derived(!st?.setUp && !st?.joining);

  $effect(() => {
    const k = (e) => { if (e.key === "Escape") { e.stopPropagation(); rooms.share = null; } };
    const d = (e) => { if (el && !el.contains(e.target) && !e.target.closest?.("[data-testid='room-glyph']")) rooms.share = null; };
    window.addEventListener("keydown", k, true);
    window.addEventListener("pointerdown", d, true);
    return () => { window.removeEventListener("keydown", k, true); window.removeEventListener("pointerdown", d, true); };
  });
</script>

<div class="pop" role="dialog" aria-label="Share this project" data-testid="share-popover" bind:this={el}>
  {#if rooms.error}<p class="err" role="alert">{rooms.error}</p>{/if}
  {#if needsSetup}
    <SetupStep />
  {:else if !st?.setUp}
    <p>Finish pairing this machine first.</p>
    <button onclick={() => { rooms.share = null; rooms.machines = "join"; }}>Machines &amp; network…</button>
  {:else}
    <h4>Share “{label}”</h4>
    <p class="dim">A room for this project. Your other Commander machines get it when they're online; colleagues only when you invite them.</p>
    <div class="row">
      <button class="primary" data-testid="share-go" disabled={rooms.busy} onclick={() => shareProject(cwd)}>Share</button>
      <button onclick={() => (rooms.share = null)}>Cancel</button>
    </div>
  {/if}
</div>

<style>
  .pop {
    position: absolute; z-index: 30; top: 100%; left: 0; margin-top: 4px; width: 260px;
    background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2);
    padding: var(--sp-2); box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35); cursor: default;
    display: flex; flex-direction: column; gap: var(--sp-2); font-size: var(--fs-1); color: var(--text-0);
  }
  h4 { margin: 0; font-size: var(--fs-1); }
  p { margin: 0; }
  .dim { color: var(--text-2); }
  .err { color: var(--crit, #d03b3b); }
  .row { display: flex; gap: var(--sp-1); }
</style>
