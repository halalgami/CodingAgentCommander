<script>
  import Drawer from "./Drawer.svelte";
  import Select from "./Select.svelte";
  import { app } from "../stores.svelte.js";
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
</style>
