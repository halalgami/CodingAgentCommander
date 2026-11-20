<script>
  // Quit guard (rooms spec §9): some of my messages are only on this machine.
  import { rooms, loadRooms } from "./rooms.svelte.js";
  import * as App from "../../../wailsjs/go/main/App.js";

  let waiting = $state(false);
  let gen = 0; // each Wait/Cancel starts a new generation; a stale loop never quits
  async function wait() {
    const mine = ++gen;
    waiting = true;
    // Keep syncing; quit on its own once nothing is queued (checks every 2 s, up to 2 min).
    for (let i = 0; i < 60; i++) {
      await new Promise((r) => setTimeout(r, 2000));
      if (mine !== gen) return;
      let n = 1;
      try { n = await App.QueuedCount(); } catch {}
      if (mine !== gen) return; // cancelled while counting (phase-1 deferred race)
      if (n === 0) { App.QuitAnyway(); return; }
    }
    waiting = false;
    loadRooms();
  }
  function cancel() { gen++; waiting = false; rooms.quit = 0; }
</script>

{#if rooms.quit}
  <div class="backdrop" role="dialog" aria-modal="true" aria-label="Messages not sent" data-testid="rooms-quitguard">
    <div class="box">
      <p><strong>{rooms.quit} message{rooms.quit === 1 ? " has" : "s have"} not left this machine.</strong></p>
      <p class="dim">No other machine in the room holds {rooms.quit === 1 ? "it" : "them"} yet. Quitting now keeps {rooms.quit === 1 ? "it" : "them"} here until Commander runs again.</p>
      {#if waiting}<p data-testid="rooms-quitguard-waiting">Waiting for a machine to come online…</p>{/if}
      <button class="primary" data-testid="rooms-quitguard-wait" disabled={waiting} onclick={wait}>Wait for a machine</button>
      <button data-testid="rooms-quitguard-quit" onclick={() => App.QuitAnyway()}>Quit anyway</button>
      <button data-testid="rooms-quitguard-cancel" onclick={cancel}>Cancel</button>
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: rgb(0 0 0 / 0.45); display: grid; place-items: center; z-index: 100; }
  .box { background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-4); max-width: 420px; display: flex; flex-direction: column; gap: var(--sp-2); }
  .dim { color: var(--text-2); }
</style>
