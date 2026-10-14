<script>
  import { onMount } from "svelte";
  import { onPaste } from "./lib/pasteRoute.js";
  import { OnFileDrop } from "../wailsjs/runtime/runtime.js";
  import { handleDrop } from "./lib/dropRoute.js";
  import { EventsOn } from "../wailsjs/runtime/runtime.js";
  import { TitleBarDoubleClick, TakeStartupErrors } from "../wailsjs/go/main/App.js";
  import { onTitlebarDblClick } from "./lib/titlebar.js";
  import { gaveUpText } from "./lib/autocont.js";
  import { app, loadAll, refresh, markFinished, reloadModels, toast } from "./lib/stores.svelte.js";
  import Sidebar from "./lib/components/Sidebar.svelte";
  import SidebarCompanion from "./lib/companion/SidebarCompanion.svelte";
  import PackGenPanel from "./lib/companion/PackGenPanel.svelte";
  import { initPackGen, gen } from "./lib/companion/packgen.svelte.js";
  import Dashboard from "./lib/dashboard/Dashboard.svelte";
  import DashNav from "./lib/dashboard/DashNav.svelte";
  import { initDash, isHome } from "./lib/dashboard/dash.svelte.js";
  import { initRaidPack } from "./lib/raid/custom.svelte.js";
  import { initMotion } from "./lib/dashboard/motion.svelte.js";
  import Terminal from "./lib/Terminal.svelte";
  import Toast from "./lib/components/Toast.svelte";
  import ProvidersDrawer from "./lib/components/ProvidersDrawer.svelte";
  import ModelsDrawer from "./lib/components/ModelsDrawer.svelte";
  import SettingsDrawer from "./lib/components/SettingsDrawer.svelte";
  import UsageDrawer from "./lib/components/UsageDrawer.svelte";
  import HistoryDrawer from "./lib/components/HistoryDrawer.svelte";
  import DocViewer from "./lib/components/DocViewer.svelte";
  import DocsDrawer from "./lib/components/DocsDrawer.svelte";
  import { openDoc, openDocsList } from "./lib/docview.svelte.js";
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
    EventsOn("session:autocont-gaveup", (e) => toast(gaveUpText(e)));
    // Errors from startup were held until now (an event then would be lost).
    try { TakeStartupErrors().then((list) => (list || []).forEach((m) => toast(m, "error"))).catch(() => {}); } catch {}
  } catch {}

  onMount(() => {
    bootOnFirstRun();
    // Subscribed at start, not when the panel opens: a run continues while
    // the panel is closed, so its state has to be current the moment it
    // reopens.
    initPackGen();
    initTheme();
    initPrefs();
    loadAll();
    initDash();
    initRaidPack();
    initMotion();
    window.__prefs = prefs; // exposed so the built-preview smoke can toggle prefs
    window.__app = app;     // ditto: the smokes seed sessions/config with no bindings
    // Leak-assertion seam: openPaneCount() is termbus's own proof that every
    // openPane() got its matching close(). Terminal.svelte lives inside
    // {#key app.sessionKey}, so it is destroyed and recreated on every session
    // switch — this is the only way a Playwright spec can see that teardown
    // actually ran across real mount/destroy cycles, not just in termbus's
    // own unit test.
    window.__termbus = { openPaneCount };
    // Same seam as __app: the wizard's state is Go-fed at runtime, so a
    // Playwright spec has no other way to drive its phases.
    window.__packgen = gen;
    // Same seam again: a dynamic import() of a .svelte.js module does not
    // resolve against the vite preview build (the module is bundled), so a
    // Playwright spec opens a document through this instead.
    window.__openDoc = openDoc;
    window.__openDocsList = openDocsList;
    // Plain-browser test runs have no window.runtime; the shell must still mount.
    try { OnFileDrop(handleDrop, true); } catch {}
    // Capture phase: xterm's own textarea handler never sees a claimed paste.
    window.addEventListener("paste", onPaste, true);
    const iv = setInterval(refresh, 5000);
    return () => { clearInterval(iv); window.removeEventListener("paste", onPaste, true); };
  });

  $effect(() => {
    document.documentElement.style.setProperty("--sidebar-w", prefs.sidebarW + "px");
    document.documentElement.style.zoom = prefs.uiScale / 100;
  });
</script>

<div class="shell">
  <!-- svelte-ignore a11y_no_static_element_interactions (double-click zoom mirrors the macOS titlebar; keyboard users have Window → Zoom) -->
  <header class="titlebar" data-testid="titlebar" style="--wails-draggable: drag" ondblclick={(e) => onTitlebarDblClick(e, () => TitleBarDoubleClick())}>
    <span class="wordmark" data-testid="wordmark">COMMANDER</span>
    <DashNav />
    <button class="palette-hint" data-testid="open-palette" onclick={() => (app.paletteOpen = true)}>
      <span>Search or jump…</span><kbd>⌘K</kbd>
    </button>
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
    <Sidebar>
      {#snippet dock()}
        {#if app.companionCfg.kind === "panel"}
          <svelte:boundary onerror={(e) => console.error("companion render failed", e)}>
            <SidebarCompanion />
            {#snippet failed()}
              <!-- Swallowed, not rethrown: a broken figure must never take the
                   deck with it. Nothing renders, the sidebar keeps its layout. -->
              <div></div>
            {/snippet}
          </svelte:boundary>
        {/if}
      {/snippet}
    </Sidebar>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (WAI-ARIA window splitter: a focusable separator resized with ←/→) -->
    <div
      class="divider" data-testid="sidebar-divider"
      role="separator" aria-orientation="vertical" aria-label="Sidebar width" aria-valuemin="240" aria-valuemax="480" tabindex="0"
      onkeydown={(e) => {
        // ←/→ resize by 16 px, within the same 240–480 px range as dragging
        const d = { ArrowLeft: -16, ArrowRight: 16 }[e.key];
        if (!d) return;
        e.preventDefault();
        const cur = parseInt(getComputedStyle(document.documentElement).getPropertyValue("--sidebar-w")) || 290;
        document.documentElement.style.setProperty("--sidebar-w", Math.min(480, Math.max(240, cur + d)) + "px");
      }}
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
        <!-- inert while home: Tab or a drawer's focus restore must never reach the hidden xterm. -->
        <div class="term-layer" data-testid="term-layer" inert={isHome()}>
          {#key app.sessionKey}<Terminal sessionKey={app.sessionKey} theme={xtermTheme} />{/key}
        </div>
      {/if}
      {#if isHome()}
        <Dashboard />
      {/if}
    </section>
  </div>
  <!-- drawers / palette / toasts mount here in Tasks 5-8 -->
  {#if app.drawer === "providers"}<ProvidersDrawer />{/if}
  {#if app.drawer === "models"}<ModelsDrawer />{/if}
  {#if app.drawer === "settings"}<SettingsDrawer />{/if}
  {#if app.drawer === "usage"}<UsageDrawer />{/if}
  {#if app.drawer === "history"}<HistoryDrawer />{/if}
  {#if app.drawer === "docview"}<DocViewer />{/if}
  {#if app.drawer === "docs"}<DocsDrawer />{/if}
  <CommandPalette />
  <LaunchConfirmModal />
  <AboutModal />
  <LitellmRuntimeModal />
  <DependenciesModal />
  <PackGenPanel />
  <Hotkeys />
  <Toast />
  <BootIntro />
  <!-- The companion now lives in a native desktop overlay window (see
       overlay_darwin.go), not in the deck. -->
</div>

<style>
  :global(:root) { --wails-drop-target: drop; }
  .pane:global(.wails-drop-target-active) { outline: 2px dashed var(--accent); outline-offset: -4px; }
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
  .palette-hint {
    --wails-draggable: no-drag;
    margin-left: var(--sp-4); display: flex; align-items: center; gap: var(--sp-2);
    background: var(--surface-2); border: 1px solid var(--border-0); border-radius: var(--r-2);
    color: var(--text-2); cursor: pointer; padding: 3px 10px; font-size: var(--fs-1);
  }
  .palette-hint:hover { color: var(--text-0); border-color: var(--accent-dim); }
  .palette-hint kbd {
    font-family: var(--font-mono); font-size: var(--fs-0); color: var(--text-1);
    background: var(--surface-3); border-radius: var(--r-1); padding: 0 4px;
  }
  .content { flex: 1; display: flex; min-height: 0; }
  .divider {
    width: 4px; cursor: col-resize; flex: none;
    background: transparent;
    transition: background var(--t-fast) var(--ease-out);
  }
  .divider:hover, .divider:active { background: var(--accent-dim); }
  .pane { flex: 1; background: var(--surface-0); min-width: 0; position: relative; }
  .term-layer { height: 100%; }
</style>
