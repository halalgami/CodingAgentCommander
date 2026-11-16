<script>
  // Al Asturlab (⌘8): the observatory. Spec §5; every figure from the Go stats engine.
  import { onMount } from "svelte";
  import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
  import { app } from "../stores.svelte.js";
  import { ast, load, currentRange, queries, exportData } from "./asturlab.svelte.js";
  import { fmtTokens, fmtUSD, fmtDelta, fmtHours } from "./format.js";
  import { stackBars, punchcard, calendar, treemap } from "./geom.js";
  import { colorOf, SEQ, MARK } from "./palette.js";
  import Chart from "./Chart.svelte";
  import ProjectPage from "./ProjectPage.svelte";
  import QuotaCard from "./QuotaCard.svelte";
  import AiCard from "./AiCard.svelte";
  import Wrapped from "./Wrapped.svelte";
  let wrapped = $state(false);
  import { quota } from "./quota.svelte.js";

  const RANGES = [["today", "Today"], ["7d", "7 d"], ["30d", "30 d"], ["all", "All"]];
  const WD = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

  const filter = $derived(ast.view?.project ? { project: ast.view.project } : ast.view?.model ? { model: ast.view.model } : {});
  const reload = () => load(filter);

  // Refresh on ledger change, at most every 10 s, only while open (spec §9).
  onMount(() => {
    reload();
    let last = 0, timer = null;
    const poke = () => {
      const wait = Math.max(0, last + 10_000 - Date.now());
      if (timer) return;
      timer = setTimeout(() => { timer = null; last = Date.now(); reload(); }, wait);
    };
    let offs = [];
    try { offs = [EventsOn("dash:project", poke), EventsOn("dash:city", poke)]; } catch {}
    return () => { clearTimeout(timer); offs.forEach((o) => typeof o === "function" && o()); };
  });

  function pick(id) { ast.range = id; reload(); }
  function setCustom() { if (ast.custom.from && ast.custom.to && ast.custom.from <= ast.custom.to) { ast.range = "custom"; reload(); } }
  function toggle(k, v) { ast[k] = v; reload(); }
  function drill(view) { ast.view = view; reload(); }

  const r = $derived(currentRange());
  const cur = $derived(ast.overview?.cur || {});
  const prev = $derived(ast.overview?.prev || {});
  const isCost = $derived(ast.unit === "cost");
  const fmtV = (v) => (isCost ? fmtUSD(v) : fmtTokens(v));
  // A row's value in the current unit; unpriced tokens are named, never shown as $0.
  const fmtRow = (x) => {
    if (!isCost || !(x.unpriced > 0)) return fmtV(x.value);
    const note = `${fmtTokens(x.unpriced)} tokens unpriced`;
    return x.value > 0 ? `${fmtUSD(x.value)} + ${note}` : `— (${note})`;
  };
  const pricedNote = $derived(`estimate · priced ${cur.pricedAt || ast.pricedAt}`);

  // tooltip per chart
  let tips = $state({});
  function tipAt(chart, e, lines) {
    const box = e.currentTarget.closest(".plot").getBoundingClientRect();
    const m = e.currentTarget.getBoundingClientRect();
    tips[chart] = { x: m.left - box.left + m.width / 2, y: m.top - box.top, lines };
  }
  const clear = (chart) => (tips[chart] = null);

  // geometry
  const SW = 600, SH = 180;
  const spend = $derived(stackBars(ast.spend, { w: SW, h: SH, pad: 2, from: r.from, to: r.to }));
  const splits = $derived([...new Set(ast.spend.map((x) => x.split))]);
  const TW = 600, TH = 220;
  const tree = $derived(treemap(ast.projects.map((p) => ({ key: p.key, weight: p.value })), { w: TW, h: TH }));
  const projByKey = $derived(new Map(ast.projects.map((p) => [p.key, p])));
  const PW = 600, PH = 175;
  const punch = $derived(punchcard(ast.punch, { w: PW, h: PH }));
  const weeks = $derived(r.days > 91 ? 52 : 13);
  const cal = $derived(calendar(ast.calendar, { to: r.to, weeks, cell: 11 }));
  const calW = $derived((Math.max(0, ...cal.map((c) => c.x)) + 11));
  const toolMax = $derived(Math.max(1, ...ast.tools.map((t) => t.value)));

  const pct = (a, b) => (b > 0 ? ((a / b) * 100).toFixed(1) + "%" : "—");
  const per = (a, b) => (b > 0 ? fmtTokens(a / b) : "—");
  const modelCost = (m) => (m.priced ? fmtUSD(m.costUSD) : "tokens only");

  const tiles = $derived([
    { id: "tokens", label: "Tokens", value: fmtTokens(cur.tokens), delta: fmtDelta(cur.tokens, prev.tokens) },
    { id: "cost", label: "API-equivalent", value: cur.costUSD > 0 ? fmtUSD(cur.costUSD) : cur.unpricedTokens > 0 ? "—" : fmtUSD(0),
      delta: fmtDelta(cur.costUSD, prev.costUSD), note: pricedNote,
      extra: cur.unpricedTokens > 0 ? `+ ${fmtTokens(cur.unpricedTokens)} tokens unpriced` : "" },
    { id: "planx", label: "Plan value", value: cur.planX > 0 ? cur.planX.toFixed(1) + "×" : "—",
      delta: fmtDelta(cur.planX, prev.planX), note: cur.planUSD > 0 ? `of ${fmtUSD(cur.planUSD)} plan (prorated)` : "set your plan in Settings" },
    { id: "hours", label: "Active", value: fmtHours(cur.activeMs), delta: fmtDelta(cur.activeMs, prev.activeMs) },
    { id: "commits", label: "Commits", value: String(cur.commits ?? 0), delta: fmtDelta(cur.commits, prev.commits) },
    { id: "cache", label: "Cache share", value: cur.cacheShare ? Math.round(cur.cacheShare * 100) + "%" : "—",
      delta: fmtDelta(cur.cacheShare, prev.cacheShare), note: cur.cacheSavedUSD > 0 ? `saved ${fmtUSD(cur.cacheSavedUSD)}` : "" },
  ]);
  const q = $derived(queries(r, filter));
</script>

<div class="ast" data-testid="asturlab">
  {#if wrapped}<Wrapped onclose={() => (wrapped = false)} />{/if}
  <div class="bar">
    <h2>Al Asturlab <span class="ar" lang="ar">الأسطرلاب</span></h2>
    {#if ast.view}
      <button class="back" data-testid="ast-back" onclick={() => drill(null)}>← All</button>
      <span class="crumb" data-testid="ast-crumb">{ast.view.label || ast.view.model}</span>
    {/if}
    <div class="ranges" role="group" aria-label="Range">
      {#each RANGES as [id, label]}
        <button data-testid={"ast-range-" + id} aria-pressed={ast.range === id} onclick={() => pick(id)}>{label}</button>
      {/each}
      <input type="date" aria-label="From" bind:value={ast.custom.from} onchange={setCustom} />
      <input type="date" aria-label="To" bind:value={ast.custom.to} onchange={setCustom} />
    </div>
    <button data-testid="ast-wrapped-open" onclick={() => (wrapped = true)}>Wrapped</button>
    {#if !ast.locked}
      <span class="exp">Export <button data-testid="ast-export-csv" onclick={() => exportData("days", "csv")}>CSV</button><button data-testid="ast-export-json" onclick={() => exportData("days", "json")}>JSON</button></span>
    {/if}
  </div>

  {#if ast.locked}
    <div class="locked" data-testid="ast-locked">
      <p>Al Asturlab reads your project history, and history is off.</p>
      <p class="dim">Live now: {app.sessions?.length ?? 0} sessions.
        {#each (quota.now?.providers || []).filter((p) => p.supported).flatMap((p) => p.windows || []) as w}<span class="pill">{w.label} {Math.round(w.util)}%</span>{/each}</p>
      <button class="primary" onclick={() => (app.drawer = "settings")}>Turn on history</button>
    </div>
  {:else}
    {#if ast.view?.project}<ProjectPage projectKey={ast.view.project} range={r} />{/if}
    <div class="tiles" hidden={!!ast.view}>
      {#each tiles as t}
        <div class="tile" data-testid={"ast-tile-" + t.id}>
          <div class="tl">{t.label}</div>
          <div class="tv">{t.value}</div>
          <div class="td" class:up={t.delta.startsWith("+")}>{t.delta} <span class="dim">vs previous</span></div>
          {#if t.note}<div class="tn">{t.note}{#if t.id === "cost" && cur.stale} <span class="stale" data-testid="ast-stale">rates may be stale</span>{/if}</div>{/if}
          {#if t.extra}<div class="tn">{t.extra}</div>{/if}
        </div>
      {/each}
    </div>

    <div class="cols">
      <div class="col">
        <Chart id="spend" title="Spend over time" sub={isCost ? pricedNote : "tokens"} tip={tips.spend}
          columns={[{ key: "key", label: "Day" }, { key: "split", label: ast.split === "model" ? "Model" : "Provider" }, { label: isCost ? "≈ USD" : "Tokens", num: true, fmt: fmtRow }]}
          rows={ast.spend} onexport={() => exportData("chart", "csv", q.spend)}>
          {#snippet legend()}
            <div class="legend">
              <span class="seg"><button aria-pressed={!isCost} onclick={() => toggle("unit", "tokens")}>Tokens</button><button aria-pressed={isCost} onclick={() => toggle("unit", "cost")}>$</button></span>
              <span class="seg"><button aria-pressed={ast.split === "model"} onclick={() => toggle("split", "model")}>Model</button><button aria-pressed={ast.split === "provider"} onclick={() => toggle("split", "provider")}>Provider</button></span>
              {#each splits as s}<span class="key"><i style:background={colorOf(s)}></i>{s}</span>{/each}
              {#if isCost && ast.stale}<span class="stale" data-testid="ast-stale">rates may be stale</span>{/if}
            </div>
          {/snippet}
          <svg viewBox="0 0 {SW} {SH}" preserveAspectRatio="none" role="img" aria-label="Spend per day">
            {#each spend.bars as b}
              <rect role="presentation" x={b.x} y={b.y} width={b.w} height={Math.max(0, b.h - 1)} rx="2" fill={colorOf(b.split)} data-value={b.value}
                onmouseenter={(e) => tipAt("spend", e, [b.key, b.split, fmtRow(b)])} onmouseleave={() => clear("spend")} />
            {/each}
          </svg>
          {#if !ast.spend.length}<p class="empty">Nothing in this range.</p>{/if}
        </Chart>

        <Chart id="treemap" title="Projects" sub="tokens · click to open" tip={tips.treemap}
          columns={[{ key: "label", label: "Project" }, { label: "Tokens", num: true, fmt: (x) => fmtTokens(x.value) }]} rows={ast.projects}
          onexport={() => exportData("chart", "csv", q.projects)}>
          <svg viewBox="0 0 {TW} {TH}" role="img" aria-label="Projects by tokens">
            {#each tree as t, i}
              {@const p = projByKey.get(t.key)}
              <g role="button" tabindex="-1" data-testid="ast-tree-cell" data-key={t.key} data-value={p?.value} style="cursor:pointer"
                onclick={() => drill({ project: t.key, label: p?.label || t.key })} onkeydown={() => {}}
                onmouseenter={(e) => tipAt("treemap", e, [p?.label || t.key, fmtTokens(p?.value)])} onmouseleave={() => clear("treemap")}>
                <rect x={t.x + 1} y={t.y + 1} width={Math.max(0, t.w - 2)} height={Math.max(0, t.h - 2)} rx="3" fill={MARK} fill-opacity={Math.max(0.25, 0.9 - i * 0.08)} />
                {#if t.w > 60 && t.h > 22}<text x={t.x + 6} y={t.y + 16} class="tlabel">{p?.label}</text>{/if}
              </g>
            {/each}
          </svg>
        </Chart>

        <Chart id="punch" title="When you work" sub="turns by weekday × hour" tip={tips.punch}
          columns={[{ label: "When", fmt: (x) => { const [w, h] = x.key.split("-"); return `${WD[+w]} ${h}:00`; } }, { label: "Turns", num: true, fmt: (x) => Math.round(x.value) }]}
          rows={ast.punch} onexport={() => exportData("chart", "csv", q.punch)}>
          <svg viewBox="-34 0 {PW + 34} {PH + 14}" role="img" aria-label="Turns by weekday and hour">
            {#each WD as d, i}<text x="-30" y={(i + 0.5) * (PH / 7) + 4} class="axis">{d}</text>{/each}
            {#each [0, 6, 12, 18] as h}<text x={(h + 0.5) * (PW / 24)} y={PH + 12} class="axis" text-anchor="middle">{h}</text>{/each}
            {#each punch as c}
              {#if c.r > 0}
                <circle role="presentation" cx={c.cx} cy={c.cy} r={Math.max(2, c.r)} fill={MARK} data-value={c.value}
                  onmouseenter={(e) => tipAt("punch", e, [`${WD[c.wd]} ${c.hr}:00`, `${Math.round(c.value)} turns`])} onmouseleave={() => clear("punch")} />
              {:else}<circle cx={c.cx} cy={c.cy} r="1" fill="var(--border-1)" />{/if}
            {/each}
          </svg>
        </Chart>
      </div>

      <div class="col">
        <QuotaCard seen={ast.providers.map((r) => r.key)} />
        <AiCard />

        <Chart id="calendar" title="Calendar" sub={`${weeks} weeks · tokens`} tip={tips.calendar}
          columns={[{ key: "key", label: "Day" }, { label: "Tokens", num: true, fmt: (x) => fmtTokens(x.value) }]} rows={ast.calendar}
          onexport={() => exportData("chart", "csv", q.calendar)}>
          <svg viewBox="0 0 {calW} 77" style:max-width={calW * 1.6 + "px"} role="img" aria-label="Tokens per day calendar">
            {#each cal as c}
              <rect role="presentation" x={c.x} y={c.y} width="9" height="9" rx="2" fill={SEQ[c.level]} data-value={c.value}
                onmouseenter={(e) => tipAt("calendar", e, [c.day, fmtTokens(c.value)])} onmouseleave={() => clear("calendar")} />
            {/each}
          </svg>
        </Chart>

        <Chart id="models" title="Models compared" sub={pricedNote}
          columns={[{ key: "model", label: "Model" }, { label: "Tokens", num: true, fmt: (m) => fmtTokens(m.tokens) }, { label: "≈ $", num: true, fmt: modelCost }]}
          rows={ast.models} onexport={() => exportData("days", "csv")}>
          <div class="tbl">
            <table>
              <thead><tr><th>Model</th><th class="num">Tokens</th><th class="num">/turn</th><th class="num">Err</th><th class="num">Compact</th><th class="num">Test fail</th><th class="num">/commit</th><th class="num">/100 lines</th><th class="num">API-eq.</th></tr></thead>
              <tbody>
                {#each ast.models as m}
                  <tr data-testid="ast-model-row" tabindex="0" onclick={() => drill({ model: m.model })} onkeydown={(e) => e.key === "Enter" && drill({ model: m.model })}>
                    <td><i class="dot" style:background={colorOf(m.model)}></i>{m.model}<span class="dim"> · {m.provider}</span></td>
                    <td class="num">{fmtTokens(m.tokens)}</td><td class="num">{per(m.tokens, m.turns)}</td>
                    <td class="num">{pct(m.errors, m.turns)}</td><td class="num">{pct(m.compactions, m.turns)}</td><td class="num">{pct(m.testFails, m.testRuns)}</td>
                    <td class="num">{per(m.tokens, m.commits)}</td><td class="num">{per(m.tokens * 100, m.lines)}</td><td class="num">{modelCost(m)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </Chart>

        <Chart id="tools" title="How the agents work" sub="tool calls" tip={tips.tools}
          columns={[{ key: "key", label: "Tool" }, { label: "Calls", num: true, fmt: (x) => Math.round(x.value) }]} rows={ast.tools}
          onexport={() => exportData("chart", "csv", q.tools)}>
          <div class="hbars">
            {#each ast.tools.slice(0, 10) as t}
              <div class="hb"><span>{t.key}</span><span class="track"><i style:width={(t.value / toolMax) * 100 + "%"} style:background={MARK}></i></span><span class="num">{Math.round(t.value)}</span></div>
            {/each}
          </div>
        </Chart>
      </div>
    </div>
  {/if}
</div>

<style>
  .ast { position: relative; padding: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-3); color: var(--text-0); min-height: 100%; box-sizing: border-box; }
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-3); }
  h2 { margin: 0; font-size: var(--fs-3); font-weight: 600; }
  .ar { color: var(--text-2); font-weight: 400; margin-left: 6px; }
  .ranges { display: flex; gap: 4px; align-items: center; margin-left: auto; }
  .ranges button, .seg button, .exp button, .back { background: var(--surface-2); border: 1px solid var(--border-0); color: var(--text-1); border-radius: var(--r-1); padding: 3px 9px; font-size: var(--fs-0); cursor: pointer; }
  button[aria-pressed="true"] { background: var(--accent-faint); color: var(--text-0); border-color: var(--accent-dim); }
  input[type="date"] { background: var(--surface-2); border: 1px solid var(--border-0); color: var(--text-1); border-radius: var(--r-1); font-size: var(--fs-0); padding: 2px 4px; }
  .exp { color: var(--text-2); font-size: var(--fs-0); display: flex; gap: 4px; align-items: center; }
  .crumb { font-weight: 600; }
  .tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: var(--sp-2); }
  .tile { background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-3); }
  .tl { color: var(--text-2); font-size: var(--fs-0); }
  .tv { font-size: var(--fs-4); font-weight: 600; font-variant-numeric: tabular-nums; margin: 2px 0; }
  .td { font-size: var(--fs-0); color: var(--text-1); }
  .tn { font-size: var(--fs-0); color: var(--text-2); margin-top: 2px; }
  .stale { color: var(--warn); border: 1px solid var(--warn); border-radius: var(--r-1); padding: 0 4px; font-size: var(--fs-0); }
  .cols { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr); gap: var(--sp-3); }
  @media (max-width: 1000px) { .cols { grid-template-columns: minmax(0, 1fr); } }
  .col { display: flex; flex-direction: column; gap: var(--sp-3); min-width: 0; }
  svg { width: 100%; height: auto; display: block; overflow: visible; }
  .legend { display: flex; flex-wrap: wrap; gap: var(--sp-2); align-items: center; margin-bottom: var(--sp-2); font-size: var(--fs-0); color: var(--text-1); }
  .seg { display: inline-flex; gap: 2px; }
  .key { display: inline-flex; align-items: center; gap: 4px; }
  .key i, .dot { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 4px; }
  .axis { fill: var(--text-2); font-size: 10px; }
  .tlabel { fill: #fff; font-size: 11px; pointer-events: none; }
  .empty, .dim { color: var(--text-2); }
  .hb { display: grid; grid-template-columns: 9em 1fr 3.5em; gap: var(--sp-2); align-items: center; font-size: var(--fs-0); color: var(--text-1); margin: 3px 0; }
  .track { height: 6px; background: var(--surface-2); border-radius: 3px; overflow: hidden; }
  .track i { display: block; height: 100%; background: var(--accent); border-radius: 3px; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .tbl { overflow: auto; max-height: 260px; }
  table { width: 100%; border-collapse: collapse; font-size: var(--fs-0); }
  th { text-align: left; color: var(--text-2); font-weight: 500; }
  th, td { padding: 3px 6px; border-bottom: 1px solid var(--border-0); white-space: nowrap; }
  tbody tr { cursor: pointer; }
  tbody tr:hover, tbody tr:focus-visible { background: var(--surface-2); }
  .locked { background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-4); max-width: 560px; }
  .pill { background: var(--surface-2); border-radius: var(--r-1); padding: 1px 6px; margin-left: 6px; }
  .primary { background: var(--accent); color: var(--on-accent); border: 0; border-radius: var(--r-1); padding: 6px 12px; cursor: pointer; }
</style>
