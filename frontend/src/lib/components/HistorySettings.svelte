<script>
  import { onMount } from "svelte";
  import { app, refreshLedger, setLedger, setLedgerExcluded, forgetProject, deleteLedger, pickFolder } from "../stores.svelte.js";

  onMount(refreshLedger);
  const st = $derived(app.ledger.status);
  let deleteArmed = $state(false);
  const fmt = (n) => (n ?? 0).toLocaleString("en-US");
  const hours = (ms) => (ms / 3_600_000).toFixed(1);

  async function addExcluded() {
    const before = app.folder;
    await pickFolder();
    const picked = app.folder;
    app.folder = before;
    if (picked && picked !== before) await setLedgerExcluded([...(st?.excluded ?? []), picked]);
  }
  function armDelete() {
    if (deleteArmed) { deleteArmed = false; deleteLedger(); return; }
    deleteArmed = true;
    setTimeout(() => (deleteArmed = false), 3000);
  }
</script>

<h3>History</h3>
<p class="dim" data-testid="ledger-explainer">
  Remember how much time you spend on each project with Claude. Commander keeps counts only — time,
  turns, tools, tests, commits and (optionally) session titles — never your prompts, Claude's replies
  or your code. It is stored in <span class="mono">{st?.dir}</span> and never leaves this Mac.
</p>
<label class="row">
  <input type="checkbox" data-testid="ledger-toggle" checked={!!st?.enabled}
    onchange={(e) => setLedger(e.currentTarget.checked, st?.storeTitles ?? true)} />
  Remember my project history
</label>
{#if st?.enabled}
  <label class="row">
    <input type="checkbox" data-testid="ledger-titles" checked={st.storeTitles}
      onchange={(e) => setLedger(true, e.currentTarget.checked)} />
    Keep session titles
  </label>
  {#if st.progress && !st.progress.done && st.progress.total > 0}
    <p class="dim mono" data-testid="ledger-progress">Indexing transcripts: {fmt(st.progress.files)} / {fmt(st.progress.total)}</p>
  {/if}
  {#if st.readOnly}<p class="dim">Another Commander is writing history; this one is read-only.</p>{/if}
  {#if st.lastError}<p class="err">{st.lastError}</p>{/if}

  <h4>Projects</h4>
  <ul class="projects">
    {#each app.ledger.projects as p (p.key)}
      <li>
        <span class="label" title={p.root}>{p.label}</span>
        <span class="dim mono">{hours(p.totals.activeMs)} h · {p.totals.sessions} sessions</span>
        <button class="ghost" onclick={() => forgetProject(p.key)}>Forget</button>
      </li>
    {/each}
  </ul>

  <h4>Excluded folders</h4>
  <ul class="projects">
    {#each st.excluded as ex (ex)}
      <li><span class="mono" title={ex}>{ex}</span>
        <button class="ghost" onclick={() => setLedgerExcluded(st.excluded.filter((x) => x !== ex))}>Remove</button></li>
    {/each}
  </ul>
  <button class="ghost" onclick={addExcluded}>Exclude a folder…</button>
{/if}
{#if st?.enabled || st?.hasData}
  <button class="ghost danger" data-testid="ledger-delete" onclick={armDelete}>
    {deleteArmed ? "Click again to delete all history" : "Delete all history"}
  </button>
{/if}

<style>
  .row { display: flex; gap: var(--sp-2); align-items: center; margin: var(--sp-1) 0; }
  .dim { color: var(--text-1); font-size: var(--fs-1); }
  .err { color: var(--crit); font-size: var(--fs-1); }
  h4 { font-size: var(--fs-1); color: var(--text-1); margin: var(--sp-3) 0 var(--sp-1); }
  .projects { list-style: none; padding: 0; margin: 0; display: grid; gap: 4px; }
  .projects li { display: flex; gap: var(--sp-2); align-items: center; font-size: var(--fs-1); }
  .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ghost { background: var(--surface-2); color: var(--text-1); border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 2px 8px; cursor: pointer; }
  .danger { color: var(--crit); margin-left: var(--sp-2); }
</style>
