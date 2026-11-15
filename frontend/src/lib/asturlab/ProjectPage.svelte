<script>
  // Project drill-in header (spec §5): hours, tokens per commit, code size and
  // complexity over time, hotspots, achievements. Charts below it are the
  // observatory's own, filtered to this project.
  import { ProjectMetrics, Stats } from "../../../wailsjs/go/main/App.js";
  import { dash } from "../dashboard/dash.svelte.js";
  import { fmtTokens, fmtHours } from "./format.js";
  import { MARK } from "./palette.js";

  let { projectKey, range } = $props();
  let metrics = $state({ files: {}, snapshots: [] });
  let sums = $state({ activeMs: 0, tokens: 0, commits: 0 });

  async function one(metric) {
    try { const r = await Stats({ from: range.from, to: range.to, groupBy: "project", metric, project: projectKey }); return r.rows?.[0]?.value || 0; } catch { return 0; }
  }
  $effect(() => {
    const key = projectKey, r = range; // re-run on project or range change
    void r.from; void r.to;
    ProjectMetrics(key).then((m) => (metrics = m || { files: {}, snapshots: [] })).catch(() => {});
    Promise.all([one("activeMs"), one("tokens"), one("commits")]).then(([activeMs, tokens, commits]) => (sums = { activeMs, tokens, commits }));
  });

  const snaps = $derived(metrics.snapshots || []);
  const latest = $derived(snaps.at(-1));
  const hot = $derived(Object.entries(metrics.files || {}).filter(([, f]) => f.hotspot > 0).sort((a, b) => b[1].hotspot - a[1].hotspot).slice(0, 5));
  const unlocks = $derived((dash.progress?.projects || []).find((p) => p.key === projectKey)?.unlocks || []);
  function spark(key, w = 160, h = 32) {
    if (snaps.length < 2) return "";
    const vs = snaps.map((s) => s[key] || 0), lo = Math.min(...vs), hi = Math.max(...vs), d = hi - lo || 1;
    return vs.map((v, i) => `${(i / (vs.length - 1)) * w},${h - ((v - lo) / d) * (h - 4) - 2}`).join(" ");
  }
</script>

<section class="pp" data-testid="ast-project">
  <div class="stat"><span class="k">Active</span><span class="v">{fmtHours(sums.activeMs)}</span></div>
  <div class="stat"><span class="k">Tokens</span><span class="v">{fmtTokens(sums.tokens)}</span></div>
  <div class="stat"><span class="k">Tokens / commit</span><span class="v">{sums.commits > 0 ? fmtTokens(sums.tokens / sums.commits) : "—"}</span></div>
  <div class="stat"><span class="k">Code</span><span class="v">{latest ? latest.code.toLocaleString("en-US") + " lines" : "not scanned"}</span>
    {#if spark("code")}<svg viewBox="0 0 160 32" class="sp" aria-hidden="true"><polyline points={spark("code")} fill="none" stroke={MARK} stroke-width="2" /></svg>{/if}</div>
  <div class="stat"><span class="k">Complexity</span><span class="v">{latest ? latest.complexity.toLocaleString("en-US") : "—"}</span>
    {#if spark("complexity")}<svg viewBox="0 0 160 32" class="sp" aria-hidden="true"><polyline points={spark("complexity")} fill="none" stroke={MARK} stroke-width="2" /></svg>{/if}</div>
  <div class="stat"><span class="k">Achievements</span><span class="v">{unlocks.length}</span></div>
  {#if hot.length}
    <div class="hot"><span class="k">Hotspots</span>
      <ol>{#each hot as [path, f]}<li><span class="mono">{path}</span> <span class="dim">complexity {f.complexity}</span></li>{/each}</ol>
    </div>
  {/if}
</section>

<style>
  .pp { display: flex; flex-wrap: wrap; gap: var(--sp-3); background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: var(--sp-3); }
  .stat { display: flex; flex-direction: column; min-width: 120px; }
  .k { color: var(--text-2); font-size: var(--fs-0); }
  .v { font-size: var(--fs-2); font-weight: 600; font-variant-numeric: tabular-nums; }
  .sp { width: 120px; height: 24px; }
  .hot { flex: 1 1 260px; }
  ol { margin: 2px 0 0; padding-left: 1.2em; font-size: var(--fs-0); color: var(--text-1); }
  .mono { font-family: var(--font-mono); }
  .dim { color: var(--text-2); }
</style>
