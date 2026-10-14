<script>
  import { flip } from "svelte/animate";
  import Sigil from "./Sigil.svelte";
  import { activityText } from "../activity.js";
  import { labelOf } from "./label.js";
  import { layoutLane, tributaries, waitGaps } from "./river.js";
  import { prefs, setPref } from "../prefs.svelte.js";
  let { rows, timeline, now, onopen, animate = true } = $props();

  const LANE_H = 54, TRIB_H = 14, ROW_Y = [10, 24, 38];
  const windowMs = $derived((prefs.dashRiverWindow === 5 ? 5 : 10) * 60 * 1000);
  let width = $state(600);
  const COLORS = { read: "var(--text-1)", web: "var(--text-1)", other: "var(--text-2)", edit: "var(--accent)", think: "var(--text-2)", agent: "var(--accent-hover)", shell: "var(--ok)" };

  function lanes(row) {
    const evs = timeline[row.id] || [];
    const tribs = tributaries(row.crew, evs, now, windowMs, width);
    return { glyphs: layoutLane(evs, now, windowMs, width), tribs, gaps: waitGaps(evs, row, now, windowMs, width), height: LANE_H + tribs.length * TRIB_H };
  }
</script>

<div class="river" data-testid="dash-river" class:still={!animate}>
  <div class="bar">
    <span>Last</span>
    {#each [5, 10] as m}
      <button data-testid="river-window-{m}" aria-pressed={(prefs.dashRiverWindow === 5 ? 5 : 10) === m}
        onclick={() => setPref("dashRiverWindow", m)}>{m} min</button>
    {/each}
  </div>
  {#each rows as row (row.key)}
    {@const L = lanes(row)}
    <div class="lane st-{row.activity}" class:external={row.external} data-testid="river-lane" data-key={row.key} animate:flip={{ duration: 260 }}
      tabindex="0" data-nav role="button" aria-disabled={row.external} aria-label="Open {row.title}"
      onkeydown={(e) => e.key === "Enter" && !row.external && onopen(row, e.currentTarget)}>
      <div class="label">
        <Sigil id={row.id || row.key} tint={row.tint} size={18} />
        <div class="text">
          <div class="title">{labelOf(row).project}</div>
          <div class="meta" title={labelOf(row).task}>{labelOf(row).task}</div>
        </div>
      </div>
      <div class="flow" bind:clientWidth={width}>
        <svg width="100%" height={L.height} aria-label="{row.title} activity">
          {#each ROW_Y as y}<line x1="0" x2={width} y1={y} y2={y} class="staff" />{/each}
          {#each L.gaps as g}
            <rect x={g.x0} y="2" width={Math.max(0, g.x1 - g.x0)} height={LANE_H - 8} class="gap" />
          {/each}
          {#each L.glyphs as g}
            <circle data-testid="river-glyph" cx={g.x} cy={ROW_Y[g.row]} r={g.kind === "think" ? 1.6 : 3.2} fill={COLORS[g.kind]}><title>{g.label}</title></circle>
          {/each}
          {#each L.tribs as t, i}
            {@const y = LANE_H + i * TRIB_H + 4}
            {@const x1 = t.x1 ?? width}
            {#if t.braid}
              <g data-testid="river-braid">
                <path d="M{t.x0},{ROW_Y[1]} C{t.x0 + 12},{ROW_Y[1]} {t.x0 + 12},{y} {t.x0 + 24},{y} L{x1},{y}" class="trib braid" />
                <text x={Math.min(x1 - 30, t.x0 + 30)} y={y - 3} class="braid-label">+{t.count}</text>
              </g>
            {:else}
              <g data-testid="river-trib" data-agent={t.agent} data-background={String(t.background)}>
                <path d="M{t.x0},{ROW_Y[1]} C{t.x0 + 10},{ROW_Y[1]} {t.x0 + 10},{y} {t.x0 + 20},{y} L{Math.max(t.x0 + 20, x1 - 20)},{y}
                  {t.x1 != null ? `C${x1 - 10},${y} ${x1 - 10},${ROW_Y[1]} ${x1},${ROW_Y[1]}` : ''}"
                  class="trib" class:bg={t.background} />
                {#each t.glyphs as g}<circle cx={g.x} cy={y} r="2.2" fill={COLORS[g.kind]}><title>{g.label}</title></circle>{/each}
                {#if t.x1 != null}<circle data-testid="river-merge" cx={x1} cy={ROW_Y[1]} r="3" class="merge" />{/if}
              </g>
            {/if}
          {/each}
          <circle cx={width - 4} cy={ROW_Y[1]} r="5" class="head" />
        </svg>
        <div class="now">
          {activityText({ activity: row.activity, tool: row.tool, target: row.target, hotspot: row.hotspot, unread: row.unread, delegating: row.delegating, crewOut: row.crewOut })}
        </div>
        {#each L.gaps.filter((g) => g.label) as g}
          <div class="wait" data-testid="river-wait" style="left:{g.x0}px">
            {g.label}
            {#if !row.external}<button data-testid="river-review" onclick={() => onopen(row)}>Review →</button>{/if}
          </div>
        {/each}
      </div>
    </div>
  {/each}
</div>

<style>
  .river { padding: var(--sp-3) var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-2); }
  .bar { display: flex; gap: 4px; align-items: center; color: var(--text-2); font-size: var(--fs-0); justify-content: flex-end; }
  .bar button { background: none; border: 1px solid var(--border-0); color: var(--text-1); border-radius: var(--r-1); padding: 1px 6px; cursor: pointer; font-size: var(--fs-0); }
  .bar button[aria-pressed="true"] { color: var(--text-0); border-color: var(--accent-dim); }
  .lane { display: grid; grid-template-columns: 220px 1fr; gap: var(--sp-3); align-items: start; background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 6px var(--sp-2); }
  .lane.external { opacity: 0.7; }
  .lane:focus-visible { outline: 2px solid var(--accent-dim); outline-offset: 1px; }
  .lane.st-permission { border-color: var(--warn); }
  .label { display: flex; gap: 8px; min-width: 0; }
  .text { min-width: 0; }
  .title { color: var(--text-0); font-size: var(--fs-1); font-weight: 600; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .meta { color: var(--text-2); font-size: var(--fs-0); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .flow { position: relative; min-width: 0; }
  svg { display: block; overflow: visible; }
  .staff { stroke: var(--border-0); stroke-width: 1; }
  .gap { fill: color-mix(in srgb, var(--warn) 10%, transparent); stroke: var(--warn); stroke-dasharray: 3 3; }
  .trib { fill: none; stroke: var(--accent-dim); stroke-width: 1.5; }
  .trib.bg { stroke-dasharray: 4 3; }
  .trib.braid { stroke-width: 4; }
  .braid-label { fill: var(--text-1); font-size: 10px; }
  .merge { fill: var(--accent); }
  .head { fill: var(--accent); filter: drop-shadow(0 0 4px var(--accent)); animation: pulse 2s ease-in-out infinite; }
  .st-permission .head { fill: var(--warn); }
  .now { color: var(--text-1); font-family: var(--font-mono); font-size: var(--fs-0); text-align: right; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .wait { position: absolute; top: 4px; color: var(--warn); font-size: var(--fs-0); display: flex; gap: 6px; align-items: center; padding-left: 6px; }
  .wait button { background: var(--surface-2); color: var(--text-0); border: 1px solid var(--warn); border-radius: var(--r-1); font-size: var(--fs-0); cursor: pointer; }
  .still .head { animation: none; }
  @keyframes pulse { 50% { opacity: 0.6; } }
</style>
