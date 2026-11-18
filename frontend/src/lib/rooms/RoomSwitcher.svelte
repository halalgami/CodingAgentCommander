<script>
  // Titlebar Rooms → switcher (UX spec §4): my rooms, share an open
  // session's project, join with a code, machines, closed and hidden rooms.
  import { rooms, act, openPanelFor, shareProject } from "./rooms.svelte.js";
  import SetupStep from "./SetupStep.svelte";
  let { onclose } = $props();
  let el = $state(null);

  const open = $derived(rooms.list.filter((r) => !r.closed));
  const closed = $derived(rooms.list.filter((r) => r.closed));
  // Open sessions' projects with no room yet, once per project (identity, else folder).
  const candidates = $derived.by(() => {
    const seen = new Set(), out = [];
    for (const [cwd, p] of Object.entries(rooms.projects).sort(([a], [b]) => a.localeCompare(b))) {
      const k = p.identity || p.root;
      if (p.roomId || seen.has(k)) continue;
      seen.add(k);
      out.push({ cwd, label: p.label || p.root });
    }
    return out;
  });
  const st = $derived(rooms.status);

  $effect(() => {
    const k = (e) => { if (e.key === "Escape") { e.stopPropagation(); onclose(); } };
    const d = (e) => { if (el && !el.contains(e.target) && !e.target.closest?.("[data-testid='open-rooms']")) onclose(); };
    window.addEventListener("keydown", k, true);
    window.addEventListener("pointerdown", d, true);
    return () => { window.removeEventListener("keydown", k, true); window.removeEventListener("pointerdown", d, true); };
  });
  const run = (fn) => async () => { await fn(); onclose(); };
  async function other() {
    const f = await act("PickRoomFolder");
    if (f) await shareProject(f);
  }
</script>

<div class="sw" role="dialog" aria-label="Rooms" data-testid="room-switcher" bind:this={el}>
  {#if rooms.error}<p class="err" role="alert">{rooms.error}</p>{/if}
  {#if !st?.setUp && !st?.joining}
    <SetupStep />
  {:else}
    {#if open.length}
      <h5>Rooms</h5>
      {#each open as r (r.id)}
        <button class="item" data-testid="switch-room" onclick={run(() => openPanelFor(r.id, true))}>
          <span class="nm">{r.name}</span>
          {#if r.needsFolder}<span class="amber small">needs a folder</span>{/if}
          {#if r.online}<span class="small ok">● {r.online} online</span>{/if}
          {#if r.unread}<span class="badge">{r.unread}</span>{/if}
        </button>
      {/each}
    {/if}
    {#if st?.setUp}
      <h5>Share a project</h5>
      {#each candidates as c (c.cwd)}
        <button class="item" data-testid="switch-share" title={c.cwd} onclick={run(() => shareProject(c.cwd))}>{c.label}</button>
      {:else}
        <p class="dim small">Every open session's project is shared.</p>
      {/each}
      <button class="item dim" onclick={run(other)}>Other folder…</button>
    {/if}
    <hr />
    <button class="item" data-testid="switch-join" onclick={run(() => (rooms.machines = "join"))}>Join with a code…</button>
    <button class="item" data-testid="switch-machines" onclick={run(() => (rooms.machines = "list"))}>Machines &amp; network…</button>
    {#if closed.length}
      <h5>Closed rooms</h5>
      {#each closed as r (r.id)}<button class="item dim" onclick={run(() => openPanelFor(r.id, true))}>{r.name}</button>{/each}
    {/if}
    {#if rooms.hidden.length}
      <h5>Hidden rooms</h5>
      {#each rooms.hidden as h (h.id)}
        <div class="item row"><span class="nm dim">{h.name}</span>
          {#if h.restoring}<span class="dim small" title="It comes back from one of your machines that has it, or from a new invite by the owner.">waiting for it to sync back</span>
          {:else}<button class="link" data-testid={"switch-restore-" + h.id} onclick={run(() => act("RestoreRoom", h.id))}>Restore</button>{/if}</div>
      {/each}
    {/if}
  {/if}
</div>

<style>
  .sw {
    position: absolute; right: var(--sp-2); top: calc(var(--titlebar-h) + 2px); z-index: 50; width: 280px; max-height: 70vh; overflow-y: auto;
    background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: var(--sp-2);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35); display: flex; flex-direction: column; gap: 2px; font-size: var(--fs-1); color: var(--text-0);
  }
  h5 { margin: var(--sp-1) 0 2px; font-size: var(--fs-0); color: var(--text-2); text-transform: uppercase; letter-spacing: 0.06em; }
  .item { display: flex; align-items: center; gap: 6px; background: none; border: 0; color: var(--text-0); text-align: left; padding: 5px 6px; border-radius: var(--r-1); cursor: pointer; }
  button.item:hover, button.item:focus-visible { background: var(--accent-faint); }
  .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { padding: 0 5px; border-radius: 8px; background: var(--accent, #3987e5); color: #fff; font-size: 10px; }
  .amber { color: #c98500; }
  .ok { color: var(--ok, #2fa84f); }
  .dim { color: var(--text-2); }
  .small { font-size: var(--fs-0); }
  .link { background: none; border: 0; color: var(--accent); cursor: pointer; }
  .err { color: var(--crit, #d03b3b); margin: 0; }
  hr { border: 0; border-top: 1px solid var(--border-0); width: 100%; margin: 4px 0; }
</style>
