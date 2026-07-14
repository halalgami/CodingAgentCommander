<script>
  import { onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime.js";
  import { app, loadAll, refresh, markFinished } from "./lib/stores.svelte.js";
  import Sidebar from "./lib/components/Sidebar.svelte";
  import EmptyState from "./lib/components/EmptyState.svelte";
  import Terminal from "./lib/Terminal.svelte";
  import Toast from "./lib/components/Toast.svelte";
  import ProvidersDrawer from "./lib/components/ProvidersDrawer.svelte";
  import ModelsDrawer from "./lib/components/ModelsDrawer.svelte";
  import SettingsDrawer from "./lib/components/SettingsDrawer.svelte";
  import Hotkeys from "./lib/components/Hotkeys.svelte";
  import CommandPalette from "./lib/components/CommandPalette.svelte";
  import BootIntro, { bootOnFirstRun } from "./lib/components/BootIntro.svelte";
  import { initTheme, xtermTheme } from "./lib/theme/theme.svelte.js";
  import { initPrefs, prefs, setPref } from "./lib/prefs.svelte.js";

  // Guarded: window.runtime doesn't exist in a plain browser (Playwright);
  // without this the mount aborts (the B1 blank-window bug).
  try {
    EventsOn("session:finished", (windowID) => markFinished(windowID));
  } catch {}

  onMount(() => {
    bootOnFirstRun();
    initTheme();
    initPrefs();
    loadAll();
    const iv = setInterval(refresh, 5000);
    return () => clearInterval(iv);
  });

  $effect(() => {
    document.documentElement.style.setProperty("--sidebar-w", prefs.sidebarW + "px");
    document.documentElement.style.zoom = prefs.uiScale / 100;
  });
</script>

<div class="shell">
  <header class="titlebar" data-testid="titlebar" style="--wails-draggable: drag">
    <span class="wordmark" data-testid="wordmark">COMMANDER</span>
  </header>
  <div class="content">
    <Sidebar />
    <div
      class="divider" data-testid="sidebar-divider"
      onpointerdown={(e) => {
        e.preventDefault();
        const move = (ev) => {
          const w = Math.min(480, Math.max(240, Math.round(ev.clientX)));
          document.documentElement.style.setProperty("--sidebar-w", w + "px");
        };
        const up = (ev) => {
          window.removeEventListener("pointermove", move);
          window.removeEventListener("pointerup", up);
          const w = Math.min(480, Math.max(240, Math.round(ev.clientX)));
          setPref("sidebarW", w);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up);
      }}
    ></div>
    <section class="pane">
      {#if app.sessionKey}
        {#key app.sessionKey}<Terminal sessionKey={app.sessionKey} theme={xtermTheme} />{/key}
      {:else}
        <EmptyState />
      {/if}
    </section>
  </div>
  <!-- drawers / palette / toasts mount here in Tasks 5-8 -->
  {#if app.drawer === "providers"}<ProvidersDrawer />{/if}
  {#if app.drawer === "models"}<ModelsDrawer />{/if}
  {#if app.drawer === "settings"}<SettingsDrawer />{/if}
  <CommandPalette />
  <Hotkeys />
  <Toast />
  <BootIntro />
</div>

<style>
  .shell { height: 100vh; display: flex; flex-direction: column; background: var(--surface-0); }
  .titlebar {
    height: var(--titlebar-h); flex: none; display: flex; align-items: center;
    background: var(--surface-1); border-bottom: 1px solid var(--border-0);
    padding-left: 84px; /* traffic-light inset */
    user-select: none;
  }
  .wordmark {
    font-size: var(--fs-1); font-weight: 600; letter-spacing: 0.28em;
    color: var(--text-1);
  }
  .content { flex: 1; display: flex; min-height: 0; }
  .divider {
    width: 4px; cursor: col-resize; flex: none;
    background: transparent;
    transition: background var(--t-fast) var(--ease-out);
  }
  .divider:hover, .divider:active { background: var(--accent-dim); }
  .pane { flex: 1; background: var(--surface-0); min-width: 0; }
</style>
