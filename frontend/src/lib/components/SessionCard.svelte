<script>
  import { openDocsList } from "../docview.svelte.js";
  import { groupByProvider } from "../modelGroups.js";
  import { activityText } from "../activity.js";
  import { shots, captureInto, lastScreenshotInto, attachFileInto } from "../attachments.svelte.js";

  let {
    session, stat, isActive = false, isFinished, isErrored = false,
    models, onselect, onrename, onkill, onswap, onrc,
  } = $props();
  const native = $derived(stat?.provider === "anthropic");
  // Native <select> cannot collapse, so this is grouped but always open.
  const swapGroups = $derived(groupByProvider(models));

  let renaming = $state(false);
  let renameVal = $state("");
  let killArmed = $state(false);
  let killTimer;

  let attachOpen = $state(false);
  function runAttach(fn) { attachOpen = false; fn(session.windowID); }
  // Close on Escape or a pointer press outside the menu, while it is open.
  $effect(() => {
    if (!attachOpen) return;
    const onKey = (e) => { if (e.key === "Escape") { e.stopPropagation(); attachOpen = false; } };
    const onDown = (e) => { if (!e.target.closest || !e.target.closest(".attach")) attachOpen = false; };
    window.addEventListener("keydown", onKey, true);
    window.addEventListener("pointerdown", onDown, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      window.removeEventListener("pointerdown", onDown, true);
    };
  });

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
  // meter width: % of the model's OWN context window, clamped to stay visible.
  // This used to divide by a hardcoded 200k, which pinned the meter near full
  // on the 1M-window models that are now most of the catalog.
  const meterPct = $derived.by(() => {
    if (!stat) return 0;
    const window = stat.contextWindow || 200_000;
    return Math.min(100, Math.max(2, (stat.contextTokens / window) * 100));
  });
  // Compact token count: 1000000 -> "1M", 200000 -> "200k".
  function tokens(n) {
    if (n >= 1_000_000) {
      const m = n / 1_000_000;
      return (Number.isInteger(m) ? m : m.toFixed(1)) + "M";
    }
    return Math.round(n / 1000) + "k";
  }
  const running = $derived(stat?.status === "active" && !isFinished);
  // Project folder, shown unless the window name already is the folder.
  const folder = $derived(stat?.cwd ? (stat.cwd.split("/").filter(Boolean).pop() ?? "") : "");
  const activity = $derived(activityText(stat));
</script>

<li class="card" class:active={isActive} class:finished={isFinished} class:errored={isErrored}
  data-testid="session-card" data-window-id={session.windowID} aria-current={isActive ? "true" : undefined}>
  <div class="row">
    <span class="led" class:running class:done={isFinished} class:error={isErrored}
      title={isErrored ? "Errored" : undefined} aria-hidden="true"></span>
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
      {#if isActive && shots.supported}
        <span class="attach">
          <button class="icon" data-testid="attach-{session.windowID}" aria-label="Attach to this session"
            aria-haspopup="menu" aria-expanded={attachOpen} title="Attach a screenshot or file"
            onclick={() => (attachOpen = !attachOpen)}>📎</button>
          {#if attachOpen}
            <span class="menu" role="menu">
              <button role="menuitem" onclick={() => runAttach(captureInto)}>Capture region…</button>
              <button role="menuitem" onclick={() => runAttach(lastScreenshotInto)}>Last screenshot</button>
              <button role="menuitem" onclick={() => runAttach(attachFileInto)}>Attach file…</button>
            </span>
          {/if}
        </span>
      {/if}
      <button
        class="icon" data-testid="docs-open-{session.windowID}"
        title="Documents in this project"
        onclick={() => openDocsList(stat?.cwd ?? "", session.windowID, true)}
      >📄</button>
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
  {#if stat?.title}
    <div class="title" data-testid="session-title" title={stat.title}>{stat.title}</div>
  {/if}
  {#if activity}
    <div class="activity mono" class:attn={stat?.activity === "permission"} data-testid="session-activity">{activity}</div>
  {/if}
  {#if folder && folder !== session.name}
    <div class="folder mono" title={stat.cwd} data-testid="session-folder">{folder}</div>
  {/if}
  {#if stat}
    <div class="meter" aria-hidden="true">
      <div class="fill {stat.band}" style="width: {meterPct}%"></div>
    </div>
    <div class="telemetry">
      <span class="mono">{tokens(stat.contextTokens)} / {tokens(stat.contextWindow || 200_000)}</span>
      <span class="detail mono">{stat.turns}t · {Math.floor(stat.uptimeSeconds / 60)}m</span>
      {#if stat.autoContinues > 0}<span class="auto mono" data-testid="card-autocont" title="Auto-continued {stat.autoContinues} times on this prompt">↻ {stat.autoContinues}</span>{/if}
      {#if stat.remoteControl}<span class="rc" title="Remote control enabled">📱</span>{/if}
    </div>
    <div class="detail more">
      <span class="badge mono">{stat.model} · {stat.provider}</span>
      <select
        class="swap" data-testid="swap-select"
        onchange={(e) => { if (e.target.value) onswap(session.windowID, e.target.value); e.target.value = ""; }}
      >
        <option value="" selected>swap…</option>
        {#each swapGroups as g (g.provider)}
          {#if g.single}
            {#each g.models as m (m.id)}
              <option value={m.id}>{m.displayLabel}</option>
            {/each}
          {:else}
            <optgroup label={g.label}>
              {#each g.models as m (m.id)}
                <option value={m.id}>{m.displayLabel}</option>
              {/each}
            </optgroup>
          {/if}
        {/each}
      </select>
    </div>
  {/if}
</li>

<style>
  .attach { position: relative; }
  .menu {
    position: absolute; top: 100%; right: 0; z-index: var(--layer-chrome);
    display: flex; flex-direction: column; min-width: 160px; padding: var(--sp-1);
    background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-2);
    box-shadow: 0 8px 24px oklch(0% 0 0 / 0.4);
  }
  .menu button {
    background: none; border: 0; color: var(--text-0); text-align: left; cursor: pointer;
    padding: var(--sp-1) var(--sp-2); border-radius: var(--r-1); font-size: var(--fs-1);
  }
  .menu button:hover, .menu button:focus-visible { background: var(--accent-faint); }
  .card:global(.wails-drop-target-active) { outline: 2px dashed var(--accent); outline-offset: -2px; }
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
  /* Errored outranks finished: a session can be both (it errored after a
     prior finish), and "something went wrong" is the more urgent fact. Same
     idiom as .finished — a persistent tint, not a transient toast — because
     the toast that reported it is long gone by the time anyone looks here. */
  .card.errored { background: oklch(30% 0.05 25); border-color: var(--crit); }

  .row { display: flex; align-items: center; gap: var(--sp-2); }
  .led {
    width: 8px; height: 8px; border-radius: 50%; flex: none;
    background: var(--text-2);
  }
  .led.running { background: var(--ok); animation: pulse 2.4s ease-in-out infinite; }
  .led.done { background: var(--accent); }
  .led.error { background: var(--crit); animation: none; }
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

  .auto { color: var(--accent); }
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
  .title { font-size: var(--fs-1); color: var(--text-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 2px; }
  .activity { font-size: var(--fs-0); color: var(--text-1); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .activity.attn { color: var(--warn); }
</style>
