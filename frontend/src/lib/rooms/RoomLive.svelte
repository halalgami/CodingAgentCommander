<script>
  // Live strip (rooms spec §11): one line per running session in the room,
  // from automatic status events. Data only — nothing here reaches a prompt.
  import { liveLine } from "./rooms.js";
  let { live = [] } = $props();
  let open = $state(true);
</script>

{#if live.length}
  <div class="live" data-testid="room-live">
    <button class="head" aria-expanded={open} onclick={() => (open = !open)}>
      <span>{live.length} session{live.length === 1 ? "" : "s"} running</span><span aria-hidden="true">{open ? "▾" : "▸"}</span>
    </button>
    {#if open}
      {#each live as l (l.device + l.session)}
        <div class="row" data-testid="room-live-row" class:off={!l.online} title={l.summary || ""}>
          <span class="dot" class:busy={l.state === "working"} class:attn={l.state === "needs approval" || l.state === "errored"} aria-hidden="true"></span>
          <span class="who">{l.machine}{l.mine ? " (here)" : ""}</span>
          <span class="what">{liveLine(l)}</span>
        </div>
      {/each}
    {/if}
  </div>
{/if}

<style>
  .live { border-bottom: 1px solid var(--border-0); padding: 2px var(--sp-2) 4px; font-size: var(--fs-0); }
  .head { display: flex; justify-content: space-between; width: 100%; background: none; border: 0; color: var(--text-2); cursor: pointer; padding: 2px 0; }
  .row { display: flex; gap: 6px; align-items: center; min-width: 0; padding: 1px 0; }
  .row.off { opacity: 0.55; }
  .who { flex: none; color: var(--text-1); }
  .what { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-2); }
  .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--text-2); }
  .dot.busy { background: var(--accent); }
  .dot.attn { background: #c98500; }
</style>
