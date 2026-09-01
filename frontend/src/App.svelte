<script>
  import { onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime.js";
  import { app, loadAll, refresh, markFinished, reloadModels, toast } from "./lib/stores.svelte.js";
  import Sidebar from "./lib/components/Sidebar.svelte";
  import EmptyState from "./lib/components/EmptyState.svelte";
  import Terminal from "./lib/Terminal.svelte";
  import Toast from "./lib/components/Toast.svelte";
  import ProvidersDrawer from "./lib/components/ProvidersDrawer.svelte";
  import ModelsDrawer from "./lib/components/ModelsDrawer.svelte";
  import SettingsDrawer from "./lib/components/SettingsDrawer.svelte";
  import UsageDrawer from "./lib/components/UsageDrawer.svelte";
  import HistoryDrawer from "./lib/components/HistoryDrawer.svelte";
  import LaunchConfirmModal from "./lib/components/LaunchConfirmModal.svelte";
  import AboutModal from "./lib/components/AboutModal.svelte";
  import LitellmRuntimeModal from "./lib/components/LitellmRuntimeModal.svelte";
  import DependenciesModal from "./lib/components/DependenciesModal.svelte";
  import Hotkeys from "./lib/components/Hotkeys.svelte";
  import CommandPalette from "./lib/components/CommandPalette.svelte";
  import BootIntro, { bootOnFirstRun } from "./lib/components/BootIntro.svelte";
  import { initTheme, xtermTheme } from "./lib/theme/theme.svelte.js";
  import { initPrefs, prefs, setPref } from "./lib/prefs.svelte.js";
  import { openPaneCount } from "./lib/termbus.js";

  // Guarded: window.runtime doesn't exist in a plain browser (Playwright);
  // without this the mount aborts (the B1 blank-window bug).
  try {
    EventsOn("session:finished", (windowID) => markFinished(windowID));
    EventsOn("menu:about", () => (app.about = true));
    EventsOn("models:updated", () => reloadModels());
    EventsOn("app:error", (msg) => toast(msg, "error"));
  } catch {}

  onMount(() => {
    bootOnFirstRun();
    initTheme();
    initPrefs();
    loadAll();
    window.__prefs = prefs; // exposed so the built-preview smoke can toggle prefs
    window.__app = app;     // ditto: the smokes seed sessions/config with no bindings
    // Leak-assertion seam: openPaneCount() is termbus's own proof that every
    // openPane() got its matching close(). Terminal.svelte lives inside
    // {#key app.sessionKey}, so it is destroyed and recreated on every session
    // switch — this is the only way a Playwright spec can see that teardown
    // actually ran across real mount/destroy cycles, not just in termbus's
    // own unit test.
    window.__termbus = { openPaneCount };
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
    <nav class="nav" data-testid="titlebar-nav">
      <button data-testid="open-history" onclick={() => (app.drawer = "history")}>History</button>
      <button data-testid="open-about" onclick={() => (app.about = true)}>About</button>
      <button data-testid="open-providers" onclick={() => (app.drawer = "providers")}>Providers</button>
      <button data-testid="open-models" onclick={() => (app.drawer = "models")}>Models</button>
      <button data-testid="open-usage" onclick={() => (app.drawer = "usage")}>Usage</button>
      <button data-testid="open-settings" onclick={() => (app.drawer = "settings")}>Settings</button>
    </nav>
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
  {#if app.drawer === "usage"}<UsageDrawer />{/if}
  {#if app.drawer === "history"}<HistoryDrawer />{/if}
  <CommandPalette />
  <LaunchConfirmModal />
  <AboutModal />
  <LitellmRuntimeModal />
  <DependenciesModal />
  <Hotkeys />
  <Toast />
  <BootIntro />
</div>

<style>
  .shell { height: 100vh; display: flex; flex-direction: column; background: var(--surface-0); }
  .titlebar {
    height: var(--titlebar-h); flex: none; display: flex; align-items: center;
    background: var(--surface-1); border-bottom: 1px solid var(--border-0);
    padding-left: 84px;              /* traffic-light inset — never remove */
    padding-right: var(--sp-2);
    user-select: none;
  }
  .wordmark {
    font-size: var(--fs-1); font-weight: 600; letter-spacing: 0.28em;
    color: var(--text-1);
  }
  /* margin-left:auto right-aligns the nav at ANY window width, so it can never
     collide with the 84px inset above. */
  .nav { margin-left: auto; display: flex; gap: 2px; }
  /* The titlebar is a Wails drag region. A control inside a drag region does not
     receive clicks until it opts out, and the failure is silent — the button
     simply drags the window instead of firing. */
  .nav button {
    --wails-draggable: no-drag;
    background: none; border: 0; color: var(--text-1); cursor: pointer;
    font-size: var(--fs-1); padding: 4px 8px; border-radius: var(--r-1);
    white-space: nowrap;
  }
  .nav button:hover { color: var(--text-0); background: var(--surface-2); }
  .content { flex: 1; display: flex; min-height: 0; }
  .divider {
    width: 4px; cursor: col-resize; flex: none;
    background: transparent;
    transition: background var(--t-fast) var(--ease-out);
  }
  .divider:hover, .divider:active { background: var(--accent-dim); }
  .pane { flex: 1; background: var(--surface-0); min-width: 0; }
</style>
