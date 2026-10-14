<!-- frontend/src/lib/fleet/CicMode.svelte — Fleet CIC DRADIS plot (phase 7b spec §4). All text is DOM text. -->
<script>
  import { onMount } from "svelte";
  import CicGlyph from "./CicGlyph.svelte";
  import { RINGS, R, CX, CY, polar, sectorsOf, contactsOf, labelSize, labelAll, ordersOf, callsign, commsOf, statusOf, cicLabels, sectorLabel } from "./cic.js";
  import { shipOf, encounterOf, wingOf, CLASSES } from "./fleet.js";
  import { battleOf } from "../raid/battle.js";
  import { raid } from "../raid/custom.svelte.js";
  import { isHome } from "../dashboard/dash.svelte.js";
  let { rows, timeline = {}, now, animate = true, onopen } = $props();

  const sectors = $derived(sectorsOf(rows));
  const contacts = $derived(contactsOf(rows, sectors));
  const clsLine = (row) => { const s = shipOf(row, raid.pack); return `${s.cls.toUpperCase()} · ${String(row.model || "model").slice(0, 20)}${row.external ? " · external" : ""}`; };
  const taskLine = (row) => String(row.title || "").slice(0, 40);
  // encounter marker sits at (x+4, y-26): diamond ±5 plus HP bar 20 wide below it
  const encBoxes = $derived(contacts.filter((c) => c.ring !== "quiet").map((c) => [c.x - 8, c.y - 33, c.x + 16, c.y - 15]));
  let selected = $state(null);
  const orders = $derived(ordersOf(contacts, selected));
  const labels = $derived(cicLabels(contacts, sectors, encBoxes, (row) => labelSize([callsign(row), clsLine(row), taskLine(row)]), (row) => labelSize([callsign(row)])));
  const comms = $derived(commsOf(rows, timeline, now));
  const status = $derived(statusOf(rows));
  const hiddenIn = (s) => contacts.filter((c) => labels.get(c.key)?.mode !== "full" && (c.row.projectKey || c.row.project || "?") === s.key).length;
  const RING_LABEL = { needs: "AWAITING ORDERS", working: "ENGAGED", quiet: "HOLDING" };
  const els = {};

  const byAngle = $derived([...contacts].sort((a, b) => a.angle - b.angle || a.ring.localeCompare(b.ring)));
  function cycle(step) {
    if (!byAngle.length) return;
    const i = byAngle.findIndex((c) => c.key === selected);
    selected = byAngle[(i + step + byAngle.length) % byAngle.length].key;
  }
  const open = (key) => { const c = contacts.find((x) => x.key === key); if (c && c.row.windowID) onopen(c.row, els[key]); };
  // Keys belong to CIC only when focus is on the page itself or inside the plot:
  // never while typing, on a focused button/link, or inside a dialog/drawer.
  function busy(t) {
    if (t?.closest?.("input, textarea, select, [contenteditable], .xterm, button, a, [role=dialog]")) return true;
    const a = document.activeElement;
    // The dashboard region itself (focused after a mode pick) counts as the map.
    return !!a && a !== document.body && !a.closest?.(".cic") && !a.matches?.('[data-testid="dash-home"]');
  }
  onMount(() => {
    const onkey = (e) => {
      if (!isHome() || e.metaKey || e.ctrlKey || e.altKey || busy(e.target)) return;
      if (e.key === "ArrowRight") { cycle(1); e.preventDefault(); }
      else if (e.key === "ArrowLeft") { cycle(-1); e.preventDefault(); }
      else if (e.key === "Enter" && selected && contacts.some((c) => c.key === selected)) { open(selected); e.preventDefault(); }
      else if (e.key === "Escape") selected = null;
    };
    window.addEventListener("keydown", onkey);
    return () => window.removeEventListener("keydown", onkey);
  });

  const ticks = Array.from({ length: 36 }, (_, i) => i * 10);
  const arc = (a0, a1, r) => { const p0 = polar(a0, r), p1 = polar(a1, r); return `M${CX},${CY} L${p0.x},${p0.y} A${r},${r} 0 ${a1 - a0 > 180 ? 1 : 0} 1 ${p1.x},${p1.y} Z`; };
  const sweep = arc(-28, 0, R);
  const ordersAt = $derived(orders ? contacts.find((c) => c.key === orders) : null);
  // The orders box is docked top-left of the plot area. When the plot is about
  // as wide as its area, the box would cover the circle's corner: the plot then
  // moves down below the box (dock). The leader ends at the box's real corner.
  let wrap = $state(null), svgEl = $state(null), ordersEl = $state(null);
  let wrapW = $state(0), wrapH = $state(0), leaderEnd = $state({ x: 40, y: 40 });
  const BOX_W = 300, BOX_H = 90;
  const dock = $derived(!!ordersAt && (wrapW - Math.min(wrapW, wrapH)) / 2 < BOX_W + 12);
  $effect(() => {
    if (!wrap) return;
    const ro = new ResizeObserver(() => { wrapW = wrap.clientWidth; wrapH = wrap.clientHeight; });
    ro.observe(wrap);
    return () => ro.disconnect();
  });
  $effect(() => {
    void wrapW; void wrapH; void dock; void ordersAt;
    if (!ordersEl || !svgEl) return;
    queueMicrotask(() => {
      const r = ordersEl?.getBoundingClientRect(), m = svgEl?.getScreenCTM?.();
      if (!r || !m) return;
      const p = svgEl.createSVGPoint();
      p.x = r.right; p.y = r.bottom;
      const q = p.matrixTransform(m.inverse());
      leaderEnd = { x: q.x, y: q.y };
    });
  });
</script>

<div class="cic" data-testid="dash-cic">
  <div class="plotwrap" class:dock bind:this={wrap}>
    <svg class="plot" bind:this={svgEl} viewBox="0 0 1000 1000" data-testid="cic-plot" role="img" aria-label="Fleet tactical plot">
      <circle cx={CX} cy={CY} r={R} class="bezel" />
      {#each ticks as d (d)}
        {@const a = polar(d, R)}{@const b = polar(d, R + (d % 30 ? 8 : 14))}
        <line x1={a.x} y1={a.y} x2={b.x} y2={b.y} class="tick" />
        {#if d % 30 === 0}{@const t = polar(d, R + 30)}<text x={t.x} y={t.y + 4} class="bearing" text-anchor="middle">{String(d).padStart(3, "0")}</text>{/if}
      {/each}
      {#each Object.entries(RINGS) as [band, f] (band)}
        <circle cx={CX} cy={CY} r={f * R} class="ring" data-testid="cic-ring" data-ring={band} />
        <text x={CX} y={CY - f * R - 6} class="ringlabel" text-anchor="middle">{RING_LABEL[band]}</text>
      {/each}
      {#if sectors.length > 1}
        {#each sectors as s (s.key)}
          {@const p = polar(s.start, R)}<line x1={CX} y1={CY} x2={p.x} y2={p.y} class="divider" data-testid="cic-divider" />
        {/each}
      {/if}
      {#each sectors as s (s.key)}
        {@const mid = sectors.length === 1 ? 180 : (s.start + s.end) / 2}{@const t = polar(mid, R - 22)}{@const more = hiddenIn(s)}{@const L = sectorLabel(s)}
        <g data-testid="cic-sector" data-key={s.key}>
          <title>SECTOR {s.label.toUpperCase()} · {s.count} ship{s.count === 1 ? "" : "s"} · {s.needs} awaiting</title>
          {#if L.title}<text x={t.x} y={t.y} class="sector" text-anchor="middle" data-testid="cic-sector-title">{L.title}</text>{/if}
          {#if L.sub}<text x={t.x} y={t.y + 14} class="sector sub" text-anchor="middle">{L.sub}{#if more}<tspan data-testid="cic-sector-more"> · +{more}</tspan>{/if}</text>{/if}
        </g>
      {/each}
      <g class="sweep" class:spin={animate} data-testid="cic-sweep"><path d={sweep} /></g>

      {#each contacts as c (c.key)}
        {@const s = shipOf(c.row, raid.pack)}{@const w = wingOf(c.row.crew)}
        {#each w.craft as k, i (k.id)}
          {@const b = { x: c.x - 26 + i * 9, y: c.y + 22 }}
          <line x1={c.x} y1={c.y} x2={b.x} y2={b.y} class="link" />
          <circle cx={b.x} cy={b.y} r="2.6" fill={s.tint} data-testid="cic-blip" />
        {/each}
        {#if w.more > 0}<text x={c.x + 12} y={c.y + 26} class="more" data-testid="cic-blip-more">+{w.more}</text>{/if}
        {#if c.ring !== "quiet"}
          {@const e = encounterOf(c.row, null)}{@const hp = battleOf(c.row, timeline[c.row.id] || [], now).hp}
          <g transform="translate({c.x + 4},{c.y - 26})" data-testid="cic-encounter" data-kind={e.kind}>
            <path d="M0,-5 L5,0 L0,5 L-5,0 Z" class="enc" />
            <rect x="-10" y="8" width="20" height="2.5" class="hpbg" /><rect x="-10" y="8" width={(20 * hp) / 100} height="2.5" class="hp" />
          </g>
        {/if}
        <g bind:this={els[c.key]} transform="translate({c.x},{c.y})" class="contact" role="button" tabindex="-1"
          data-testid="cic-contact" data-key={c.key} data-ring={c.ring} data-ship={s.id} data-selected={String(selected === c.key)}
          onclick={() => (selected = c.key)} ondblclick={() => open(c.key)} onkeydown={() => {}}>
          {#if c.row.activity === "error"}<path d="M0,-13 L3,-9 L8,-10 L8,-5 L13,0 L8,5 L8,10 L3,9 L0,13 L-3,9 L-8,10 L-8,5 L-13,0 L-8,-5 L-8,-10 L-3,-9 Z" class="breach" />
          {:else if c.ring === "needs"}<circle r="12" class="await" />{/if}
          {#if selected === c.key}<circle r="15" class="sel" />{/if}
          <rect x="-12" y="-12" width="24" height="24" fill="transparent" />
          <title>{callsign(c.row)} · {String(c.row.title || "")}</title>
          <CicGlyph ship={s.id} tint={s.tint} dashed={c.row.external} />
        </g>
        {@const L = labels.get(c.key)}
        {#if L && L.mode === "full"}
          <text x={c.x + L.dx} y={c.y + L.dy - 9} text-anchor={L.anchor} class="label" data-testid="cic-label">
            <tspan class="cs" fill={s.tint}>{callsign(c.row)}</tspan>
            <tspan x={c.x + L.dx} dy="14" class="cls">{clsLine(c.row)}</tspan>
            <tspan x={c.x + L.dx} dy="14" class="task">{taskLine(c.row)}</tspan>
          </text>
        {:else if L}
          <text x={c.x + L.dx} y={c.y + L.dy + 4} text-anchor={L.anchor} class="label cs-only" fill={s.tint} data-testid="cic-cs">{callsign(c.row)}</text>
        {/if}
        {#if c.ring === "needs" && c.key !== orders}
          <text x={c.x} y={c.y + 24} text-anchor="middle" class="pill" data-testid="cic-pill">ORDERS</text>
        {/if}
      {/each}
      {#if ordersAt}<line x1={ordersAt.x} y1={ordersAt.y} x2={leaderEnd.x} y2={leaderEnd.y} class="leader" data-testid="cic-leader" />{/if}
    </svg>
    {#if ordersAt}
      {@const s = shipOf(ordersAt.row, raid.pack)}
      <!-- docked in the plot's top-left corner (outside the circle) so it never covers a contact -->
      <div class="orders" bind:this={ordersEl} data-testid="cic-orders" data-key={ordersAt.key}>
        <div class="ohead">AWAITING ORDERS, ADMIRAL</div>
        <div class="oship">{callsign(ordersAt.row)} · {s.cls.toUpperCase()}</div>
        <div class="otask">{String(ordersAt.row.title || "").slice(0, 60)}</div>
        {#if ordersAt.row.windowID}<button data-testid="cic-open" onclick={() => open(ordersAt.key)}>OPEN</button>{/if}
      </div>
    {/if}
  </div>

  <aside class="col">
    <section>
      <h2>FLEET STATUS</h2>
      <div class="counts">
        <div><b class="amber" data-testid="cic-status-awaiting">{status.awaiting}</b><span>AWAITING ORDERS</span></div>
        <div><b data-testid="cic-status-engaged">{status.engaged}</b><span>ENGAGED</span></div>
        <div><b data-testid="cic-status-holding">{status.holding}</b><span>HOLDING</span></div>
      </div>
      <div class="hostiles">HOSTILES <b data-testid="cic-status-hostiles">{status.hostiles}</b></div>
    </section>
    <section data-testid="cic-legend">
      <h2>SHIP CLASSES</h2>
      <ul class="legend">
        {#each Object.entries(CLASSES) as [id, cls] (id)}
          <li><svg viewBox="-10 -10 20 20" width="20" height="20"><CicGlyph ship={id} tint="#9fe8a8" /></svg>{cls}<span>{id}</span></li>
        {/each}
      </ul>
    </section>
    <section class="commsec">
      <h2>COMMS</h2>
      <ul class="comms" data-testid="cic-comms">
        {#each comms as m, i (i)}<li data-tone={m.tone}><span class="clk">{m.clock}</span><span class="msg" title={m.text}>{m.text}</span></li>{/each}
        {#if !comms.length}<li class="none">No traffic</li>{/if}
      </ul>
    </section>
  </aside>
</div>

<style>
  .cic { --grn: #9fe8a8; --grn-dim: #4f7d58; --amb: #f2b84b; position: absolute; inset: 0; display: grid; grid-template-columns: 1fr 280px; background: #060b08; color: var(--grn); font-family: var(--font-mono); }
  .plotwrap { position: relative; min-width: 0; min-height: 0; display: flex; align-items: center; justify-content: center; padding: 12px; }
  .plotwrap.dock { padding-top: 102px; }
  .plot { width: 100%; height: 100%; max-height: 100%; aspect-ratio: 1; overflow: visible; }
  .bezel { fill: rgba(40, 90, 50, 0.08); stroke: var(--grn-dim); stroke-width: 1.5; }
  .tick { stroke: var(--grn-dim); stroke-width: 1; }
  .bearing, .ringlabel, .sector { fill: var(--grn-dim); font-size: 11px; letter-spacing: 0.08em; }
  .sector { fill: var(--amb); }
  .sector.sub { fill: var(--grn-dim); font-size: 10px; }
  .ring { fill: none; stroke: var(--grn-dim); stroke-width: 1; stroke-dasharray: 3 5; }
  .divider { stroke: var(--grn-dim); stroke-width: 1; opacity: 0.6; }
  .sweep path { fill: rgba(120, 220, 140, 0.12); }
  .sweep { transform-origin: 500px 500px; transform-box: view-box; }
  .sweep.spin { animation: sweep 8s linear infinite; }
  .contact { cursor: pointer; }
  .await { fill: none; stroke: var(--amb); stroke-width: 1.5; }
  .breach { fill: none; stroke: #ff5b4d; stroke-width: 1.4; }
  .sel { fill: none; stroke: #fff; stroke-width: 1; stroke-dasharray: 2 2; }
  .link { stroke: var(--grn-dim); stroke-width: 1; stroke-dasharray: 3 3; }
  .more { fill: var(--grn-dim); font-size: 10px; }
  .enc { fill: none; stroke: #ff5b4d; stroke-width: 1.4; }
  .hpbg { fill: rgba(255, 91, 77, 0.25); }
  .hp { fill: #ff5b4d; }
  .label { font-size: 12px; }
  .label .cs, .cs-only { font-weight: 700; letter-spacing: 0.06em; }
  .label .cls { fill: var(--grn-dim); font-size: 10px; }
  .label .task { fill: #cfe9d3; font-size: 11px; }
  .leader { stroke: var(--amb); stroke-width: 1; stroke-dasharray: 4 4; opacity: 0.7; }
  .label, .pill, .sweep, .leader, .link, .more, [data-testid="cic-encounter"] { pointer-events: none; }
  .pill { fill: var(--amb); font-size: 10px; letter-spacing: 0.1em; }
  .orders { position: absolute; left: 12px; top: 12px; min-width: 220px; max-width: 300px; padding: 8px 10px; background: rgba(30, 22, 6, 0.92); border: 1px solid var(--amb); color: var(--amb); font-size: 11px; display: grid; gap: 3px; z-index: 2; }
  .ohead { letter-spacing: 0.12em; font-size: 10px; }
  .oship { font-weight: 700; }
  .otask { color: #f5e2b8; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .orders button { justify-self: start; background: none; color: var(--amb); border: 1px solid var(--amb); padding: 2px 10px; cursor: pointer; font-family: inherit; letter-spacing: 0.1em; }
  .col { border-left: 1px solid #16301c; padding: 14px 16px; display: flex; flex-direction: column; gap: 16px; min-height: 0; overflow: hidden; }
  h2 { margin: 0 0 6px; font-size: 11px; letter-spacing: 0.16em; color: var(--grn-dim); font-weight: 400; }
  .counts { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px; }
  .counts b { display: block; font-size: 26px; font-weight: 400; color: var(--grn); }
  .counts b.amber { color: var(--amb); }
  .counts span { font-size: 9px; color: var(--grn-dim); letter-spacing: 0.08em; }
  .hostiles { font-size: 10px; color: var(--grn-dim); margin-top: 6px; }
  .legend { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 6px; font-size: 11px; }
  .legend li { display: flex; align-items: center; gap: 6px; }
  .legend span { color: var(--grn-dim); font-size: 9px; margin-left: auto; }
  .commsec { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  /* Fills the rest of the column and scrolls vertically only; a message wraps
     (breaking inside long paths) to at most two lines, full text on hover. */
  .comms { list-style: none; margin: 0; padding: 0; flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; font-size: 11px; display: flex; flex-direction: column; gap: 6px; }
  .comms li { display: grid; grid-template-columns: 34px minmax(0, 1fr); gap: 6px; flex: none; }
  .comms .msg { overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .comms .clk { color: var(--grn-dim); }
  .comms [data-tone="amber"] .msg { color: var(--amb); }
  .comms [data-tone="red"] .msg { color: #ff5b4d; }
  .comms [data-tone="cyan"] .msg { color: #7fe3ff; }
  .comms .none { color: var(--grn-dim); }
  @media (max-width: 900px) { .cic { grid-template-columns: 1fr; } .col { display: none; } }
  @keyframes sweep { to { transform: rotate(360deg); } }
</style>
