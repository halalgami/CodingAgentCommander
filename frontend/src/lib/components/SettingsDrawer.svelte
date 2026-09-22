<script>
  import Drawer from "./Drawer.svelte";
  import Select from "./Select.svelte";
  import { openPackGen } from "../companion/packgen.svelte.js";
  import { app, companionSetKind, companionPickPack, companionClearPack } from "../stores.svelte.js";
  import { theme, applyAccent, resetAccent } from "../theme/theme.svelte.js";
  import { prefs, setPref } from "../prefs.svelte.js";
  import { replayIntro } from "./BootIntro.svelte";

  const scrollbackOpts = [1000, 5000, 10000, 50000].map((n) => ({ value: n, label: n >= 1000 ? `${n / 1000}k lines` : `${n}` }));
  const maxColsOpts = [
    { value: 100, label: "100 cols" }, { value: 120, label: "120 cols" },
    { value: 140, label: "140 cols" }, { value: 0, label: "unlimited" },
  ];
  const uiScaleOpts = [90, 100, 110, 125].map((n) => ({ value: n, label: `${n}%` }));
</script>

<Drawer title="SETTINGS" testid="drawer-settings" onclose={() => (app.drawer = null)}>
  <h3>Accent</h3>
  <div class="swatch" style="background: var(--accent)"></div>
  <label>
    Hue <span class="mono">{Math.round(theme.h)}°</span>
    <input type="range" min="0" max="360" step="1" value={theme.h}
      data-testid="accent-hue" oninput={(e) => applyAccent(Number(e.target.value), theme.c)} />
  </label>
  <label>
    Vividness <span class="mono">{theme.c.toFixed(3)}</span>
    <input type="range" min="0.02" max="0.17" step="0.005" value={theme.c}
      data-testid="accent-chroma" oninput={(e) => applyAccent(theme.h, Number(e.target.value))} />
  </label>
  <button data-testid="accent-reset" onclick={resetAccent}>Reset to Amber Deck</button>

  <h3>Terminal</h3>
  <label>
    Font size <span class="mono">{prefs.fontSize}px</span>
    <input type="range" min="11" max="20" step="1" value={prefs.fontSize}
      data-testid="font-size" oninput={(e) => setPref("fontSize", Number(e.target.value))} />
  </label>
  <label class="row">Scrollback
    <Select testid="scrollback-select" options={scrollbackOpts} value={prefs.scrollback}
      onchange={(v) => setPref("scrollback", v)} />
  </label>
  <label class="row">Width cap
    <Select testid="maxcols-select" options={maxColsOpts} value={prefs.maxCols}
      onchange={(v) => setPref("maxCols", v)} />
  </label>

  <h3>Layout</h3>
  <label class="row">UI scale
    <Select testid="uiscale-select" options={uiScaleOpts} value={prefs.uiScale}
      onchange={(v) => setPref("uiScale", v)} />
  </label>
  <p class="dim">Sidebar width: <span class="mono">{prefs.sidebarW}px</span> — drag the divider.</p>

  <h3>Sessions</h3>
  <label class="check">
    <input type="checkbox" data-testid="rc-auto-toggle" checked={prefs.rcAutoEnable}
      onchange={(e) => setPref("rcAutoEnable", e.target.checked)} />
    Enable remote control on launch
    <span class="dim">native Anthropic sessions only</span>
  </label>

  <h3>Companion</h3>
  <!-- Mutually exclusive, and the exclusivity is enforced in Go at
       overlay-creation time: a frontend-only check would allow two pollers
       to exist during the transition (spec §6.1). -->
  <div class="radios" role="radiogroup" aria-label="Companion kind">
    <label class="check">
      <input type="radio" name="companion-kind" data-testid="companion-kind-panel"
        checked={app.companionCfg.kind === "panel"}
        onchange={() => companionSetKind("panel")} />
      Sidebar companion <span class="dim">image pack, below the session list</span>
    </label>
    <label class="check">
      <input type="radio" name="companion-kind" data-testid="companion-kind-off"
        checked={app.companionCfg.kind === "off"}
        onchange={() => companionSetKind("off")} />
      Off
    </label>
  </div>

  {#if app.companionCfg.kind === "panel"}
    <label class="row">Pack
      <span class="dim mono">
        {app.companionPack?.name || (app.companionCfg.packPath
          ? app.companionCfg.packPath.split("/").pop()
          : "none")}
      </span>
    </label>
    <div class="row">
      <!-- Generating opens a NON-MODAL panel, so this drawer closing does not
           interrupt anything: the run lives in Go. -->
      <button class="primary" data-testid="companion-create-pack"
        onclick={() => { app.drawer = null; openPackGen(); }}>
        Create a pack…
      </button>
      <button class="ghost" data-testid="companion-pick-pack" onclick={() => companionPickPack()}>
        Choose folder…
      </button>
      <button class="ghost" data-testid="companion-clear-pack" onclick={() => companionClearPack()}>
        Clear
      </button>
    </div>
    {#if app.companionWarnings.length}
      <!-- Go returns {pack, warnings[]} and never fails the app; the list lives
           in Go so it survives this drawer being closed at load time (§5.10). -->
      <ul class="warnings" data-testid="companion-warnings">
        {#each app.companionWarnings as w, i (i)}<li>{w}</li>{/each}
      </ul>
    {/if}
    <p class="dim">
      A pack is a folder with a <span class="mono">manifest.json</span> and portrait
      images, in <span class="mono">Commander/packs/</span>. Format:
      <span class="mono">docs/companion-pack-format.md</span>
    </p>
  {/if}

  <label class="check">
    <input type="checkbox" data-testid="ambient-motion-toggle" checked={prefs.ambientMotion}
      onchange={(e) => setPref("ambientMotion", e.target.checked)} />
    Ambient motion
    <span class="dim">off leaves transition-only motion</span>
  </label>
  <label>
    Notice dwell <span class="mono">{prefs.noticeSeconds.toFixed(1)}s</span>
    <input type="range" min="2" max="15" step="0.5" value={prefs.noticeSeconds}
      data-testid="notice-seconds"
      oninput={(e) => setPref("noticeSeconds", Number(e.target.value))} />
  </label>
  <label class="check">
    <input type="checkbox" data-testid="scanlines-toggle" checked={prefs.scanlines}
      onchange={(e) => setPref("scanlines", e.target.checked)} />
    Scanlines
    <span class="dim">off by default</span>
  </label>

  <footer>
    <p class="dim">Commander — Claude Code fleet control</p>
    <button class="ghost" data-testid="replay-intro" onclick={() => { app.drawer = null; replayIntro(); }}>
      Replay intro
    </button>
  </footer>
</Drawer>

<style>
  h3 {
    font-size: var(--fs-1); letter-spacing: 0.1em; color: var(--text-1);
    margin: var(--sp-4) 0 var(--sp-2); text-transform: uppercase;
  }
  h3:first-of-type { margin-top: 0; }
  .swatch { height: 40px; border-radius: var(--r-2); margin-bottom: var(--sp-3); border: 1px solid var(--border-0); }
  label { display: block; margin-bottom: var(--sp-3); font-size: var(--fs-1); color: var(--text-1); }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
  .check { display: flex; align-items: center; gap: var(--sp-2); }
  .radios { display: flex; flex-direction: column; gap: var(--sp-2); margin-bottom: var(--sp-3); }
  .radios .check { margin-bottom: 0; }
  .warnings {
    margin: 0 0 var(--sp-3); padding-left: var(--sp-4);
    color: var(--warn); font-size: var(--fs-0); line-height: 1.5;
  }
  .mono { font-family: var(--font-mono); color: var(--text-0); }
  input[type="range"] { width: 100%; margin-top: var(--sp-2); accent-color: var(--accent); }
  button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  button:hover { background: var(--surface-3); }
  footer {
    margin-top: var(--sp-5); padding-top: var(--sp-3); border-top: 1px solid var(--border-0);
    display: flex; justify-content: space-between; align-items: center;
  }
  .dim { color: var(--text-2); font-size: var(--fs-0); margin: 0; }
  .ghost { background: none; border: 0; color: var(--text-1); }
  .primary { background: var(--accent); color: var(--surface-0); border-color: var(--accent); }
  .primary:hover { background: var(--accent); filter: brightness(1.08); }
</style>
