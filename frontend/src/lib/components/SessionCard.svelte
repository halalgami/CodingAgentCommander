<script>
  let { session, stat, isActive = false, isFinished, models, onselect, onrename, onkill, onswap, onrc } = $props();
  const native = $derived(stat?.provider === "anthropic");

  let renaming = $state(false);
  let renameVal = $state("");
  let killArmed = $state(false);
  let killTimer;

  function startRename() { renaming = true; renameVal = session.name; }
  function commitRename() {
    if (renameVal.trim()) onrename(session.windowID, renameVal.trim());
    renaming = false;
  }
  function armKill() {
    if (killArmed) { clearTimeout(killTimer); killArmed = false; onkill(session.windowID); return; }
    killArmed = true;
    killTimer = setTimeout(() => (killArmed = false), 3000);
  }
  // meter width: % of a 200k context window, clamped to stay visible
  const meterPct = $derived(
    stat ? Math.min(100, Math.max(2, stat.contextTokens / 2000)) : 0,
  );
  const running = $derived(stat?.status === "active" && !isFinished);
  // Project folder, shown unless the window name already is the folder.
  const folder = $derived(stat?.cwd ? (stat.cwd.split("/").filter(Boolean).pop() ?? "") : "");
</script>

<li class="card" class:active={isActive} class:finished={isFinished} data-testid="session-card"
  aria-current={isActive ? "true" : undefined}>
  <div class="row">
    <span class="led" class:running class:done={isFinished} aria-hidden="true"></span>
    {#if renaming}
      <input
        class="rename" data-testid="rename-input" bind:value={renameVal}
        onkeydown={(e) => { if (e.key === "Enter") commitRename(); if (e.key === "Escape") renaming = false; }}
        onblur={commitRename}
      />
    {:else}
      <button class="name" data-testid="session-name" onclick={() => onselect(session.windowID)}>
        {session.name}
      </button>
    {/if}
    <span class="actions">
      {#if stat && !stat.remoteControl}
        <button class="icon" data-testid="rc-button" disabled={!native}
          title={native ? "Enable remote control (phone handoff)" : "Remote control needs a native Anthropic session"}
          onclick={() => native && onrc(session.windowID)}>📱</button>
      {/if}
      <button class="icon" title="Rename" onclick={startRename}>✎</button>
      <button
        class="icon kill" class:armed={killArmed} data-testid="kill-button"
        title={killArmed ? "Click again to confirm" : "Close session"} onclick={armKill}
      >{killArmed ? "confirm?" : "✕"}</button>
    </span>
  </div>
  {#if folder && folder !== session.name}
    <div class="folder mono" title={stat.cwd} data-testid="session-folder">{folder}</div>
  {/if}
  {#if stat}
    <div class="meter" aria-hidden="true">
      <div class="fill {stat.band}" style="width: {meterPct}%"></div>
    </div>
    <div class="telemetry">
      <span class="mono">{Math.round(stat.contextTokens / 1000)}k</span>
      <span class="mono">${stat.estCostPerTurn.toFixed(2)}/turn</span>
      <span class="detail mono">{stat.turns}t · {Math.floor(stat.uptimeSeconds / 60)}m</span>
      {#if stat.remoteControl}<span class="rc" title="Remote control enabled">📱</span>{/if}
    </div>
    <div class="detail more">
      <span class="badge mono">{stat.model} · {stat.provider}</span>
      <select
        class="swap" data-testid="swap-select"
        onchange={(e) => { if (e.target.value) onswap(session.windowID, e.target.value); e.target.value = ""; }}
      >
        <option value="" selected>swap…</option>
        {#each models as m (m.id)}
          <option value={m.id}>{m.label}</option>
        {/each}
      </select>
    </div>
  {/if}
</li>

<style>
  .card {
    background: var(--surface-2); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: var(--sp-2) var(--sp-3);
    margin-bottom: var(--sp-2); list-style: none;
    transition: border-color var(--t-fast) var(--ease-out), background var(--t-med) var(--ease-out);
  }
  .card:hover { border-color: var(--border-1); }
  /* On deck = accent left edge + lifted surface, visible at a glance without
     shouting over the finished tint. */
  .card.active {
    border-color: var(--accent-dim);
    box-shadow: inset 3px 0 0 var(--accent);
    background: var(--surface-3);
  }
  /* Finished = persistent surface tint + solid LED (readable across a room). */
  .card.finished { background: var(--accent-faint); border-color: var(--accent-dim); }

  .row { display: flex; align-items: center; gap: var(--sp-2); }
  .led {
    width: 8px; height: 8px; border-radius: 50%; flex: none;
    background: var(--text-2);
  }
  .led.running { background: var(--ok); animation: pulse 2.4s ease-in-out infinite; }
  .led.done { background: var(--accent); }
  @keyframes pulse { 50% { opacity: 0.35; } }
  @media (prefers-reduced-motion: reduce) { .led.running { animation: none; } }

  .name {
    flex: 1; text-align: left; background: none; border: 0; cursor: pointer;
    color: var(--text-0); font-size: var(--fs-2); padding: 0;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .rename {
    flex: 1; background: var(--surface-1); color: var(--text-0);
    border: 1px solid var(--accent-dim); border-radius: var(--r-1); padding: 2px 6px;
  }
  .actions { display: flex; gap: 2px; opacity: 0; transition: opacity var(--t-fast); }
  .card:hover .actions, .actions:focus-within { opacity: 1; }
  .icon {
    background: none; border: 0; color: var(--text-1); cursor: pointer;
    padding: 2px 4px; border-radius: var(--r-1); font-size: var(--fs-1);
  }
  .icon:hover { color: var(--text-0); background: var(--surface-3); }
  .icon:disabled { opacity: 0.35; cursor: default; }
  .kill.armed { color: var(--crit); font-weight: 600; }

  .meter {
    height: 3px; background: var(--surface-1); border-radius: 2px;
    margin-top: var(--sp-2); overflow: hidden;
  }
  .fill { height: 100%; border-radius: 2px; transition: width var(--t-med) var(--ease-out), background var(--t-med); }
  .fill.green { background: var(--ok); }
  .fill.amber { background: var(--warn); }
  .fill.red { background: var(--crit); }

  .telemetry {
    display: flex; gap: var(--sp-3); margin-top: var(--sp-1);
    font-size: var(--fs-0); color: var(--text-1);
  }
  .mono { font-family: var(--font-mono); }
  .folder {
    margin-top: 2px; font-size: var(--fs-0); color: var(--text-2);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .rc { color: var(--accent); }
  .detail { opacity: 0; transition: opacity var(--t-fast); }
  .card:hover .detail, .card:focus-within .detail { opacity: 1; }
  .more { display: flex; justify-content: space-between; align-items: center; margin-top: var(--sp-1); }
  .badge { font-size: var(--fs-0); color: var(--text-2); }
  .swap {
    background: var(--surface-1); color: var(--text-1); font-size: var(--fs-0);
    border: 1px solid var(--border-0); border-radius: var(--r-1); padding: 1px 4px;
  }
</style>
