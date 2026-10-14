<!-- frontend/src/lib/fleet/SectorMode.svelte — Sector Map (phase 7c spec §4). Stations by rank, docked flagships, orders strip. -->
<script>
  import { onMount } from "svelte";
  import { MAP_W, MAP_H, SHIP_W, SHIP_H, CS_DY, systemsOf, layoutSystems, subOf, stationOf, ordersStrip, chipText, headerOf, offMapOf } from "./sector.js";
  import { callsign } from "./cic.js";
  import { shipOf, CLASSES, ENCOUNTERS } from "./fleet.js";
  import { stationArt, spaceArt, shipArt, encounterArt } from "./art.js";
  import { bandOf } from "../dashboard/rows.js";
  import { raid } from "../raid/custom.svelte.js";
  import { isHome } from "../dashboard/dash.svelte.js";
  let { rows, projects = [], progress = null, now, animate = true, onopen } = $props();

  const FONT = 16;
  const locked = $derived(!progress || progress.locked);
  const systems = $derived(systemsOf(progress, projects, rows));
  const layout = $derived(layoutSystems(systems, FONT));
  const placed = $derived(layout.placed);
  const labels = $derived(layout.labels);
  const sub = subOf;
  const strip = $derived(ordersStrip(rows));
  const head = $derived(headerOf(systems, rows));
  const first = (u) => (Array.isArray(u) ? u[0] : u);
  const LADDER = [["Outpost", "< 10 h"], ["Settlement", "10–50 h"], ["Station", "50–150 h"], ["Citadel", "150–400 h"], ["Capital", "400 h+"]];

  let selected = $state(null);
  const byX = $derived([...placed].sort((a, b) => a.x - b.x || a.y - b.y));
  function cycle(step) {
    if (!byX.length) return;
    const i = byX.findIndex((p) => p.key === selected);
    selected = byX[(i + step + byX.length) % byX.length].key;
  }
  const els = {};
  const open = (row) => { if (row?.windowID) onopen(row, els[row.key]); };
  // Same rule as CIC: keys belong to the map only when focus is on the page or inside it.
  function busy(t) {
    if (t?.closest?.("input, textarea, select, [contenteditable], .xterm, button, a, [role=dialog]")) return true;
    const a = document.activeElement;
    // The dashboard region itself (focused after a mode pick) counts as the map.
    return !!a && a !== document.body && !a.closest?.(".sector") && !a.matches?.('[data-testid="dash-home"]');
  }
  onMount(() => {
    const onkey = (e) => {
      if (!isHome() || e.metaKey || e.ctrlKey || e.altKey || busy(e.target)) return;
      if (e.key === "ArrowRight") { cycle(1); e.preventDefault(); }
      else if (e.key === "ArrowLeft") { cycle(-1); e.preventDefault(); }
      else if (e.key === "Enter" && selected) {
        const p = placed.find((x) => x.key === selected);
        if (p?.g.ships[0]) { open(p.g.ships[0].row); e.preventDefault(); }
      } else if (e.key === "Escape") selected = null;
    };
    window.addEventListener("keydown", onkey);
    return () => window.removeEventListener("keydown", onkey);
  });
</script>

<div class="sector" data-testid="dash-sector" style="--galaxy: url({spaceArt('galaxy')})">
  <header class="head">
    <span class="title">SECTOR MAP</span>
    <span class="cnt amber"><b data-testid="sector-count-awaiting">{head.awaiting}</b> awaiting</span>
    <span class="cnt blue"><b data-testid="sector-count-engaged">{head.engaged}</b> engaged</span>
    <span class="cnt grey"><b data-testid="sector-count-docked">{head.docked}</b> docked</span>
    <span class="hours" data-testid="sector-hours">{head.hours} h invested across {head.systems} active system{head.systems === 1 ? "" : "s"}{#if offMapOf(rows)}<span data-testid="sector-offmap"> · +{offMapOf(rows)} off-map</span>{/if}</span>
    {#if locked}<span class="hint" data-testid="sector-hint">Enable the ledger to grow stations</span>{/if}
  </header>
  {#if strip.length}
    <div class="strip" data-testid="sector-orders">
      {#each strip as r (r.key)}
        <button class="chip" data-testid="sector-chip" data-key={r.key} onclick={() => open(r)}>{chipText(r, now)}</button>
      {/each}
    </div>
  {/if}
  <svg class="map" viewBox="0 0 {MAP_W} {MAP_H}" preserveAspectRatio="xMidYMid meet" data-testid="sector-map" role="img" aria-label="Sector map">
    {#each placed as p (p.key)}
      {@const s = p.g.station}{@const L = labels.get(p.key)}
      <g data-testid="sector-system" data-key={p.key} data-rank={p.sys.rank} data-selected={String(selected === p.key)}>
        {#if s.halo}<circle cx={s.x} cy={s.y} r={s.size * 0.62} class="halo" data-testid="sector-halo" />{/if}
        {#if selected === p.key}<circle cx={s.x} cy={s.y} r={s.size * 0.7} class="sel" />{/if}
        <g class="st" role="button" tabindex="-1" onclick={() => (selected = p.key)} onkeydown={() => {}}>
          <title>{p.sys.label} · {sub(p.sys)}</title>
          <image href={stationArt(s.art)} x={s.x - s.size / 2} y={s.y - s.size / 2} width={s.size} height={s.size}
            preserveAspectRatio="xMidYMid meet" data-testid="sector-station" data-art={s.art} />
        </g>
        {#if L && L.mode === "full"}
          <text x={p.x + L.dx} y={p.y + L.dy - 3} text-anchor={L.anchor} class="label" data-testid="sector-label">
            <tspan class="name">{p.sys.label}</tspan>
            <tspan x={p.x + L.dx} dy={FONT * 1.25} class="sub">{sub(p.sys)}</tspan>
          </text>
        {:else if L}
          <text x={p.x + L.dx} y={p.y + L.dy + 5} text-anchor={L.anchor} class="label name" data-testid="sector-name-only">{p.sys.label}</text>
        {/if}
        {#each p.g.ships as d (d.row.key)}
          {@const sh = shipOf(d.row, raid.pack)}{@const band = bandOf(d.row)}{@const dmg = d.row.activity === "error"}
          <g bind:this={els[d.row.key]} class="ship" role="button" tabindex="-1" data-testid="sector-ship" data-key={d.row.key}
            data-ship={sh.id} data-state={band} data-damaged={String(dmg)} onclick={() => open(d.row)} onkeydown={() => {}}>
            <title>{callsign(d.row)} · {String(d.row.title || "")}</title>
            {#if band === "needs"}<ellipse cx={d.x} cy={d.y} rx={SHIP_W / 2 + 5} ry={SHIP_H / 2 + 5} class="await" class:pulse={animate} data-testid="sector-await" />{/if}
            <image href={first(shipArt(sh.id, dmg))} x={d.x - SHIP_W / 2} y={d.y - SHIP_H / 2} width={SHIP_W} height={SHIP_H} preserveAspectRatio="xMidYMid meet" />
            {#if band === "working"}<path d="M{d.x + SHIP_W / 2 + 2},{d.y - 14} l5,5 l-5,5 l-5,-5 Z" class="enemy" data-testid="sector-enemy" />{/if}
            {#if band === "needs"}<text x={d.x} y={d.y + CS_DY} text-anchor="middle" class="cs" fill={sh.tint} data-testid="sector-callsign">{callsign(d.row)}</text>{/if}
          </g>
        {/each}
        {#if p.g.moreAt}<text x={p.g.moreAt.x} y={p.g.moreAt.y} class="more" data-testid="sector-more">+{p.more}</text>{/if}
      </g>
    {/each}
  </svg>
  <footer class="legend" data-testid="sector-legend">
    <div class="grp">{#each LADDER as [rank, hrs] (rank)}{@const st = stationOf(rank)}<span class="item"><img src={stationArt(st.art)} alt="" style="height:{Math.round(26 * st.scale)}px" />{rank}<em>{hrs}</em></span>{/each}</div>
    <div class="grp">{#each Object.entries(CLASSES) as [id, cls] (id)}<span class="item"><img src={first(shipArt(id, false))} alt="" />{cls}</span>{/each}</div>
    <div class="grp">{#each ENCOUNTERS as e (e.kind)}<span class="item"><img src={encounterArt(e.kind)} alt="" />{e.name}</span>{/each}</div>
  </footer>
</div>

<style>
  .sector { --amb: #f2b84b; --blue: #7fb8ff; --dim: #8a93a8; position: absolute; inset: 0; display: flex; flex-direction: column; color: #e6e9f2; font-family: var(--font-mono);
    background: linear-gradient(rgba(5, 6, 12, 0.6), rgba(5, 6, 12, 0.6)), #05060c var(--galaxy) center / cover no-repeat; }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 14px; padding: 10px 16px; background: linear-gradient(#05060ccc, #05060c00); font-size: 12px; }
  .title { letter-spacing: 0.2em; color: var(--dim); }
  .cnt b { font-size: 18px; font-weight: 400; margin-right: 4px; }
  .amber { color: var(--amb); } .blue { color: var(--blue); } .grey { color: var(--dim); }
  .hours { margin-left: auto; color: var(--dim); }
  .hint { color: var(--amb); }
  .strip { display: flex; gap: 8px; padding: 0 16px 8px; overflow-x: auto; }
  .chip { flex: none; background: rgba(40, 28, 6, 0.85); color: var(--amb); border: 1px solid var(--amb); padding: 3px 10px; font: inherit; font-size: 11px; cursor: pointer; white-space: nowrap; }
  .map { flex: 1; width: 100%; min-height: 0; }
  .st { cursor: pointer; }
  .halo { fill: rgba(255, 210, 110, 0.12); stroke: #ffd36e; stroke-width: 2; }
  .sel { fill: none; stroke: #fff; stroke-width: 1.5; stroke-dasharray: 4 4; }
  .label { font-size: 16px; }
  .label .name, .label.name { fill: #f3f5fb; font-weight: 700; }
  .label .sub { font-size: 13px; }
  .ship { cursor: pointer; }
  .await { fill: none; stroke: var(--amb); stroke-width: 2; }
  .await.pulse { animation: pulse 1.6s ease-in-out infinite; }
  .enemy { fill: none; stroke: #ff5b4d; stroke-width: 1.6; }
  .cs { font-size: 13px; font-weight: 700; letter-spacing: 0.06em; }
  .more { fill: var(--dim); font-size: 14px; }
  .label, .more, .halo, .sel { pointer-events: none; }
  /* dark halo so text reads over the bright galaxy core */
  .label, .cs, .more { paint-order: stroke; stroke: #05060c; stroke-width: 4px; stroke-linejoin: round; }
  .label .sub { fill: #b9c0d0; }
  .ship image { filter: drop-shadow(0 0 3px #000) drop-shadow(0 0 6px rgba(160, 200, 255, 0.5)); }
  .legend { display: flex; flex-wrap: wrap; gap: 6px 22px; padding: 8px 16px; background: linear-gradient(#05060c00, #05060cdd); font-size: 10px; color: var(--dim); }
  .grp { display: flex; flex-wrap: wrap; gap: 4px 12px; align-items: center; }
  .item { display: inline-flex; align-items: center; gap: 4px; }
  .item img { height: 18px; }
  .item em { font-style: normal; opacity: 0.7; }
  @keyframes pulse { 50% { opacity: 0.35; } }
</style>
