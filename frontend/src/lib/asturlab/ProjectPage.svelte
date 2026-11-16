<script>
  // Project drill-in header (spec §5): hours, tokens per commit, code size and
  // complexity over time, hotspots, achievements. Charts below it are the
  // observatory's own, filtered to this project.
  import { ProjectMetrics, Stats, ProjectSessions } from "../../../wailsjs/go/main/App.js";
  import { dash } from "../dashboard/dash.svelte.js";
  import { fmtTokens, fmtHours } from "./format.js";
  import { MARK } from "./palette.js";
  import { miniTower } from "./geom.js";

  let { projectKey, range } = $props();
  let metrics = $state({ files: {}, snapshots: [] });
  let sums = $state({ activeMs: 0, tokens: 0, commits: 0 });
  let sessions = $state([]);
  let maxTokens = $state(0);

  async function one(metric) {
    try { const r = await Stats({ from: range.from, to: range.to, groupBy: "project", metric, project: projectKey }); return r.rows?.[0]?.value || 0; } catch { return 0; }
  }
  $effect(() => {
    const key = projectKey, r = range; // re-run on project or range change
    void r.from; void r.to;
    ProjectMetrics(key).then((m) => (metrics = m || { files: {}, snapshots: [] })).catch(() => {});
    Promise.all([one("activeMs"), one("tokens"), one("commits")]).then(([activeMs, tokens, commits]) => (sums = { activeMs, tokens, commits }));
    ProjectSessions(key, r.from, r.to).then((s) => (sessions = s || [])).catch(() => (sessions = []));
    Stats({ from: r.from, to: r.to, groupBy: "project", metric: "tokens" }).then((x) => (maxTokens = Math.max(0, ...(x.rows || []).map((y) => y.value)))).catch(() => {});
  });

  const snaps = $derived(metrics.snapshots || []);
  const latest = $derived(snaps.at(-1));
  const hot = $derived(Object.entries(metrics.files || {}).filter(([, f]) => f.hotspot > 0).sort((a, b) => b[1].hotspot - a[1].hotspot).slice(0, 5));
  const unlocks = $derived((dash.progress?.projects || []).find((p) => p.key === projectKey)?.unlocks || []);
  const tower = $derived(miniTower(sums.tokens, maxTokens));
  const when = (ms) => (ms ? new Date(ms).toLocaleString("en-US", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }) : "—");
  function spark(key, w = 160, h = 32) {
    if (snaps.length < 2) return "";
    const vs = snaps.map((s) => s[key] || 0), lo = Math.min(...vs), hi = Math.max(...vs), d = hi - lo || 1;
    return vs.map((v, i) => `${(i / (vs.length - 1)) * w},${h - ((v - lo) / d) * (h - 4) - 2}`).join(" ");
  }
</script>

<section class="pp" data-testid="ast-project">
  <svg class="tower" viewBox="0 0 40 124" width="30" height="93" data-testid="ast-tower" data-h={tower.h} role="img" aria-label="Al Modon tower, {tower.floors} floors">
    <rect x="8" y={124 - tower.h} width="24" height={tower.h} rx="2" fill={MARK} opacity={tower.alpha} />
    {#each Array(tower.floors) as _, i}<rect x="13" y={124 - 9 - i * 12} width="14" height="4" rx="1" fill="var(--surface-0)" opacity="0.7" />{/each}
  </svg>
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
  <div class="sess" data-testid="ast-sessions"><span class="k">Sessions in this range ({sessions.length})</span>
    {#if sessions.length}
      <table>
        <thead><tr><th scope="col">Started</th><th scope="col">Session</th><th scope="col" class="num">Active</th><th scope="col" class="num">Tokens</th><th scope="col" class="num">Commits</th><th scope="col">Models</th></tr></thead>
        <tbody>
          {#each sessions as s (s.id)}
            <tr data-testid="ast-session-row"><td>{when(s.startedAt)}</td><td>{s.title || s.id.slice(0, 8)}</td><td class="num">{fmtHours(s.activeMs)}</td><td class="num">{fmtTokens(s.tokens)}</td><td class="num">{s.commits}</td><td class="dim">{(s.models || []).join(", ")}</td></tr>
          {/each}
        </tbody>
      </table>
    {:else}<p class="dim">No sessions in this range.</p>{/if}
  </div>
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
  .tower { align-self: flex-end; }
  .sess { flex: 1 1 100%; overflow-x: auto; }
  table { border-collapse: collapse; font-size: var(--fs-0); width: 100%; margin-top: 4px; }
  th, td { text-align: left; padding: 2px var(--sp-2) 2px 0; border-bottom: 1px solid var(--border-0); }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
