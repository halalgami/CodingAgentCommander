<script>
  // Room settings on this machine (rooms spec §11, plan R4) and agent posts
  // held by the rate limit.
  import { rooms, act } from "./rooms.svelte.js";
  let { v, onclose } = $props();
  let summaries = $state(v?.settings?.summaries ?? true);
  let budget = $state(v?.settings?.budget ?? 5);
  let rate = $state(v?.settings?.rate ?? 30);
  let el = $state(null);
  const opener = document.activeElement;
  rooms.error = ""; // an earlier action's error doesn't belong here
  $effect(() => { el?.querySelector("input")?.focus(); });
  function close() { onclose?.(); opener?.focus?.(); }
  async function save() {
    await act("SetRoomSettings", v.id, { summaries, budget: Number(budget), rate: Number(rate) });
    if (!rooms.error) close();
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="scrim" role="dialog" aria-modal="true" aria-label="Room settings" tabindex="-1" data-testid="room-settings"
  onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); close(); } }} bind:this={el}>
  <div class="box">
    <h3>Settings for {v.name} on this machine</h3>
    <label class="check"><input type="checkbox" bind:checked={summaries} data-testid="room-settings-summaries" />
      Share finish summaries — the first 600 characters of a session's last answer go into the room when it finishes.</label>
    <label>Automatic deliveries per thread (agents waiting on a reply from your own machines)
      <input type="number" min="1" max="50" bind:value={budget} data-testid="room-settings-budget" /></label>
    <label>Agent posts per hour from this machine (more are held for you to release)
      <input type="number" min="1" max="300" bind:value={rate} data-testid="room-settings-rate" /></label>
    {#if rooms.error}<p class="err" role="alert">{rooms.error}</p>{/if}
    <div class="row">
      <button onclick={close}>Cancel</button>
      <button class="primary" data-testid="room-settings-save" onclick={save}>Save</button>
    </div>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; z-index: 60; background: rgba(0, 0, 0, 0.45); display: flex; align-items: center; justify-content: center; }
  .box { width: min(480px, calc(100vw - 32px)); background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2); }
  h3 { margin: 0; font-size: var(--fs-2); overflow-wrap: anywhere; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: var(--fs-0); color: var(--text-1); }
  label.check { flex-direction: row; align-items: flex-start; gap: 6px; }
  input[type="number"] { width: 6em; }
  .row { display: flex; justify-content: flex-end; gap: var(--sp-1); }
  .err { color: var(--crit, #d03b3b); margin: 0; }
</style>
