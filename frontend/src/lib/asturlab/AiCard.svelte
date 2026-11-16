<script>
  // AI briefing + Ask your stats (spec §7). Opening the view reads the cached
  // briefing only; a model is called on the button click or the Ask Enter —
  // never on a timer or on open (owner, 2026-11-16).
  import { onMount } from "svelte";
  import { ai, loadBriefing, runBriefing, ask, weekLabel, answerBars } from "./ai.svelte.js";
  import { fmtTokens } from "./format.js";
  import { MARK } from "./palette.js";
  import Chart from "./Chart.svelte";

  onMount(loadBriefing);

  const b = $derived(ai.brief);
  const res = $derived(ai.ask?.results?.[0]);
  const q0 = $derived(ai.ask?.queries?.[0]);
  const bars = $derived(answerBars(res));
  const fmtVal = (v) => (q0?.metric === "cost" ? "$" + v.toFixed(2) : q0?.metric === "activeMs" ? (v / 36e5).toFixed(1) + " h" : fmtTokens(v));
  const rows = $derived((res?.rows || []).map((r) => ({ k: r.label || r.key, v: fmtVal(r.value) })));

  function goChart(id) {
    const el = document.querySelector(`[data-chart="${id}"]`);
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    el.setAttribute("tabindex", "-1");
    el.focus({ preventScroll: true });
  }
  function onKey(e) { if (e.key === "Enter") { e.preventDefault(); ask(); } }
</script>

<section class="ai" data-testid="ast-ai" aria-label="AI briefing">
  <header><h3>AI briefing</h3>{#if b?.week}<span class="sub">week of {weekLabel(b.week)}</span>{/if}</header>
  {#if b?.locked}
    <p class="dim small" data-testid="ast-ai-locked">Turn on history to get a weekly briefing and ask questions about your usage.</p>
  {:else if b}
    {#if b.brief}
      <p class="head" data-testid="ast-brief-headline">{b.brief.headline}</p>
      <ul class="pts">
        {#each b.brief.points as p}
          <li data-testid="ast-brief-point">
            {#if p.chart}<button class="link pt" data-testid="ast-brief-link" data-chart-link={p.chart} onclick={() => goChart(p.chart)}>{p.text}</button>{:else}{p.text}{/if}
          </li>
        {/each}
      </ul>
      <button class="link small" data-testid="ast-brief-rerun" disabled={ai.briefBusy || b.running} onclick={runBriefing}>{ai.briefBusy ? "Writing…" : "Write it again"}</button>
    {:else}
      <button class="primary" data-testid="ast-brief-run" disabled={ai.briefBusy || b.running} onclick={runBriefing}>
        {ai.briefBusy || b.running ? "Writing your briefing…" : "Click here for your weekly briefing"}
      </button>
    {/if}
    {#if b.error}<p class="err small" role="alert" data-testid="ast-brief-error">{b.error}</p>{/if}
    <p class="dim tiny" data-testid="ast-brief-disclosure">{b.disclosure}</p>

    <div class="askbox">
      <input type="text" placeholder="Ask your stats — e.g. which project cost most last month?" aria-label="Ask your stats"
        data-testid="ast-ask-input" bind:value={ai.question} onkeydown={onKey} disabled={ai.askBusy} maxlength="500" />
      <button data-testid="ast-ask-go" disabled={ai.askBusy || !ai.question.trim()} onclick={ask}>{ai.askBusy ? "…" : "Ask"}</button>
    </div>
    <p class="dim tiny" data-testid="ast-ask-disclosure">{b.askDisclosure}</p>
    {#if ai.ask}
      {#if ai.ask.error}<p class="err small" role="alert" data-testid="ast-ask-error">{ai.ask.error}</p>{/if}
      {#if ai.ask.answer}<p data-testid="ast-ask-answer">{ai.ask.answer}</p>{/if}
      {#if bars.length}
        <Chart id="ask" title="Answer data" sub={q0 ? `${q0.metric} by ${q0.groupBy}, ${q0.from} → ${q0.to}` : ""}
          columns={[{ key: "k", label: "Item" }, { key: "v", label: "Value", num: true }]} {rows}>
          <svg viewBox="0 0 400 {bars.length * 18}" class="bars" data-testid="ast-ask-bars" role="img" aria-label="Answer chart">
            {#each bars as r}
              <text x="0" y={r.y + 12} class="lbl">{r.label.length > 18 ? r.label.slice(0, 17) + "…" : r.label}</text>
              <rect x="130" y={r.y + 3} width={r.w} height="12" rx="2" fill={MARK}><title>{r.label}: {fmtVal(r.value)}</title></rect>
            {/each}
          </svg>
        </Chart>
      {/if}
    {/if}
  {/if}
</section>

<style>
  .ai { background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2); }
  header { display: flex; align-items: baseline; gap: var(--sp-2); }
  h3 { margin: 0; font-size: var(--fs-1); }
  .sub, .dim { color: var(--text-2); }
  .small { font-size: var(--fs-0); }
  .tiny { font-size: calc(var(--fs-0) * 0.9); margin: 0; }
  .head { font-weight: 600; margin: 0; }
  .pts { margin: 0; padding-left: 1.2em; display: flex; flex-direction: column; gap: 4px; }
  .pt { text-align: left; }
  .err { color: var(--danger, #d03b3b); margin: 0; }
  .askbox { display: flex; gap: var(--sp-1); }
  .askbox input { flex: 1; min-width: 0; }
  .bars { width: 100%; }
  .lbl { font-size: 11px; fill: var(--text-1); }
</style>
