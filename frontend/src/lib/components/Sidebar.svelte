<script>
  import { app, select, killSession, renameSession, swapSession, enableRemoteControl } from "../stores.svelte.js";
  import LaunchPanel from "./LaunchPanel.svelte";
  import SessionCard from "./SessionCard.svelte";
</script>

<aside data-testid="sidebar">
  <LaunchPanel />
  <ul class="sessions">
    {#each app.sessions as s (s.windowID)}
      <SessionCard
        session={s}
        stat={app.stats[s.windowID]}
        isFinished={!!app.finished[s.windowID]}
        models={app.models}
        onselect={select}
        onrename={renameSession}
        onkill={killSession}
        onswap={swapSession}
        onrc={enableRemoteControl}
      />
    {/each}
  </ul>
  <footer>
    <button data-testid="open-providers" onclick={() => (app.drawer = "providers")}>Providers</button>
    <button data-testid="open-models" onclick={() => (app.drawer = "models")}>Models</button>
    <button data-testid="open-settings" onclick={() => (app.drawer = "settings")}>Settings</button>
  </footer>
</aside>

<style>
  aside {
    width: var(--sidebar-w); flex: none; height: 100%;
    background: var(--surface-1); border-right: 1px solid var(--border-0);
    padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3);
    overflow-y: auto;
  }
  .sessions { flex: 1; margin: 0; padding: 0; overflow-y: auto; }
  footer {
    display: flex; gap: var(--sp-1); border-top: 1px solid var(--border-0);
    padding-top: var(--sp-2);
  }
  footer button {
    flex: 1; background: none; border: 0; color: var(--text-1); cursor: pointer;
    font-size: var(--fs-1); padding: 4px; border-radius: var(--r-1);
  }
  footer button:hover { color: var(--text-0); background: var(--surface-2); }
</style>
