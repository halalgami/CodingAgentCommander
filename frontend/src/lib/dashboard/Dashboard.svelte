<script>
  import { app } from "../stores.svelte.js";
  import { dash, currentMode, openRow, zoomInto, zoomOut } from "./dash.svelte.js";
  import TriageWall from "./TriageWall.svelte";
  import River from "./River.svelte";
  import CityMode from "../city/CityMode.svelte";
  import CodeCityMode from "../codecity/CodeCityMode.svelte";
  import RaidMode from "../raid/RaidMode.svelte";
  import FleetMode from "../fleet/FleetMode.svelte";
  import CicMode from "../fleet/CicMode.svelte";
  import SectorMode from "../fleet/SectorMode.svelte";
  import { prefs } from "../prefs.svelte.js";
  import { motion, motionAllowed } from "./motion.svelte.js";
  import { zoomFrom } from "./zoom.js";
  import { buildRows, sortRows } from "./rows.js";
  import EmptyState from "../components/EmptyState.svelte";

  const rows = $derived(sortRows(buildRows({ live: dash.live, windows: app.sessions, stats: app.stats, projects: dash.projects, progress: dash.progress })));

  let now = $state(Date.now());
  const animate = $derived(motionAllowed({ pref: prefs.dashMotion, reduced: motion.reduced, lowPower: dash.lowPower, visible: motion.visible, focused: motion.focused, blurredAt: motion.blurredAt, now }));
  $effect(() => {
    const iv = setInterval(() => (now = Date.now()), animate ? 1000 : 5000);
    return () => clearInterval(iv);
  });
  let homeEl;
  // Spec §3: 1–9 opens the nth session (screen order), arrows move focus.
  function onkey(e) {
    if (e.metaKey || e.ctrlKey || e.altKey || e.target.closest?.("input, textarea, select")) return;
    if (/^[1-9]$/.test(e.key)) {
      const r = rows[Number(e.key) - 1];
      if (r?.windowID) {
        e.preventDefault();
        open(r, homeEl.querySelector(`[data-key="${CSS.escape(r.key)}"]`));
      }
      return;
    }
    const dir = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[e.key];
    if (!dir) return;
    const nav = [...homeEl.querySelectorAll("[data-nav]")];
    if (!nav.length) return;
    e.preventDefault();
    const i = nav.indexOf(document.activeElement);
    nav[i < 0 ? (dir > 0 ? 0 : nav.length - 1) : (i + dir + nav.length) % nav.length].focus();
  }

  async function open(row, el) {
    await zoomFrom(el, homeEl, animate);
    await openRow(row);
  }
</script>

<!-- Keyboard delegation for the tiles/lanes inside (spec §3); each target is itself focusable. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="home" class:still={!animate} bind:this={homeEl} data-testid="dash-home" tabindex="-1" role="region" aria-label="Dashboard" onkeydown={onkey}>
  {#if rows.length === 0 && !(currentMode() === "city" && dash.city.districts.length)}
    <div data-testid="dash-empty" class="empty"><EmptyState /></div>
  {:else if currentMode() === "city"}
    {#if dash.zoom}
      <CodeCityMode zoom={dash.zoom} {rows} {now} {animate} onopen={open} onback={zoomOut} />
    {:else}
      <CityMode {rows} timeline={dash.timeline} city={dash.city} {now} {animate} onopen={open} onzoom={zoomInto} />
    {/if}
  {:else if currentMode() === "river"}
    <River {animate} {rows} timeline={dash.timeline} {now} onopen={open} />
  {:else if currentMode() === "raid"}
    <RaidMode {rows} timeline={dash.timeline} projects={dash.projects} progress={dash.progress} {now} {animate} onopen={open} />
  {:else if currentMode() === "fleet"}
    <FleetMode {rows} timeline={dash.timeline} projects={dash.projects} {now} {animate} onopen={open} />
  {:else if currentMode() === "cic"}
    <CicMode {rows} timeline={dash.timeline} {now} {animate} onopen={open} />
  {:else if currentMode() === "sector"}
    <SectorMode {rows} projects={dash.projects} progress={dash.progress} {now} {animate} onopen={open} />
  {:else}
    <TriageWall {rows} timeline={dash.timeline} {now} onopen={open} />
  {/if}
</div>

<style>
  .home { position: absolute; inset: 0; overflow: auto; background: var(--surface-0); outline: none; }
  .empty { height: 100%; }
  .home.still :global(*) { animation: none !important; transition: none !important; }
</style>
