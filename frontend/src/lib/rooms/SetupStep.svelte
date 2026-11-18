<script>
  // First-time rooms setup, inline (UX spec §5). onready runs once this
  // machine is set up as the first machine; "I already use Commander…" hands
  // over to the Machines & network dialog in join mode.
  import { rooms, act } from "./rooms.svelte.js";
  let { onready } = $props();
  let name = $state(rooms.status?.name || "");

  async function first() {
    if (!name.trim()) { rooms.error = "Type your name."; return; }
    await act("RoomsSetup", true, name.trim());
    if (rooms.status?.setUp) onready?.();
  }
  async function other() {
    await act("RoomsSetup", false, "");
    rooms.share = null;
    rooms.machines = "join";
  }
</script>

<div class="setup" data-testid="setup-step">
  <p class="lead">First, set up rooms on this machine.</p>
  <label>Your name, as others see it <input data-testid="setup-name" bind:value={name} /></label>
  <p class="dim small">This machine: <strong>{rooms.status?.machine || "this computer"}</strong></p>
  <button class="primary" data-testid="setup-first" disabled={rooms.busy} onclick={first}>This is my first Commander machine</button>
  <button data-testid="setup-other" disabled={rooms.busy} onclick={other}>I already use Commander on another machine</button>
</div>

<style>
  .setup { display: flex; flex-direction: column; gap: var(--sp-2); }
  .lead { margin: 0; font-weight: 600; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: var(--fs-1); }
  .dim { color: var(--text-2); margin: 0; }
  .small { font-size: var(--fs-0); }
</style>
