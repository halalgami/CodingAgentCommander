<script>
  import { onMount } from "svelte";
  import Drawer from "./Drawer.svelte";
  import { app, askLaunch } from "../stores.svelte.js";
  import { listProjects, pinProject, removeProject, renameProject } from "../history.js";

  const VISIBLE = 30;

  let projects = $state([]);
  let query = $state("");
  let editing = $state(""); // folder currently being renamed
  let editLabel = $state("");

  async function load() {
    try { projects = await listProjects(); } catch { projects = []; }
  }
  onMount(load);

  // Go already sorts pinned-first / recency; window the unpinned unless searching.
  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (q) {
      return projects.filter((p) => (p.folder + " " + (p.label ?? "")).toLowerCase().includes(q));
    }
    const pinned = projects.filter((p) => p.pinned);
    const recent = projects.filter((p) => !p.pinned).slice(0, VISIBLE);
    return [...pinned, ...recent];
  });

  function relTime(ms) {
    if (!ms) return "";
    const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
    if (s < 60) return "just now";
    const m = Math.round(s / 60); if (m < 60) return `${m}m ago`;
    const h = Math.round(m / 60); if (h < 24) return `${h}h ago`;
    return `${Math.round(h / 24)}d ago`;
  }

  async function togglePin(p) { try { await pinProject(p.folder, !p.pinned); } catch {} await load(); }
  async function remove(p)    { try { await removeProject(p.folder); } catch {} await load(); }
  function startRename(p)     { editing = p.folder; editLabel = p.label ?? ""; }
  async function commitRename(p) {
    try { await renameProject(p.folder, editLabel); } catch {}
    editing = ""; await load();
  }
</script>

<Drawer title="HISTORY" testid="drawer-history" onclose={() => (app.drawer = null)}>
  <input
    class="search" data-testid="history-search" placeholder="Search projects…"
    bind:value={query}
  />
  {#if !shown.length}
    <p class="dim" data-testid="history-empty">No projects yet — launch one to start your history.</p>
  {/if}
  <ul class="list" data-testid="history-list">
    {#each shown as p (p.folder)}
      <li class="row" class:missing={p.missing} data-testid="history-row-{p.folder}">
        <button class="open" data-testid="history-open-{p.folder}"
                onclick={() => askLaunch(p.folder, p.lastModelID, p.missing)}
                title="Load into the launch panel">
          <span class="name">{p.label}{#if p.pinned} 📌{/if}{#if p.missing} <span class="tag">missing</span>{/if}</span>
          <span class="path">{p.folder}</span>
          <span class="meta">{relTime(p.lastOpened)} · {p.openCount}×</span>
        </button>
        <div class="actions">
          <button data-testid="history-pin-{p.folder}" title={p.pinned ? "Unpin" : "Pin"} onclick={() => togglePin(p)}>{p.pinned ? "📌" : "📍"}</button>
          <button data-testid="history-rename-{p.folder}" title="Rename" onclick={() => startRename(p)}>✎</button>
          <button data-testid="history-remove-{p.folder}" title="Remove" onclick={() => remove(p)}>✕</button>
        </div>
        {#if editing === p.folder}
          <div class="rename">
            <input data-testid="history-rename-input-{p.folder}" bind:value={editLabel}
                   onkeydown={(e) => { if (e.key === "Enter") commitRename(p); }} />
            <button data-testid="history-rename-save-{p.folder}" onclick={() => commitRename(p)}>Save</button>
          </div>
        {/if}
      </li>
    {/each}
  </ul>
</Drawer>

<style>
  .search {
    width: 100%; background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 6px 8px; font-size: var(--fs-1); margin-bottom: var(--sp-3);
  }
  .search:focus { outline: none; border-color: var(--accent-dim); }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-1); }
  .row { display: flex; align-items: flex-start; gap: var(--sp-1); padding: 4px; border-radius: var(--r-1); flex-wrap: wrap; }
  .row:hover { background: var(--surface-2); }
  .row.missing { opacity: 0.5; }
  .open {
    flex: 1; min-width: 0; text-align: left; background: none; border: 0;
    color: var(--text-0); cursor: pointer; display: flex; flex-direction: column; gap: 2px; padding: 4px;
  }
  .name { font-size: var(--fs-1); }
  .path { font-family: var(--font-mono); font-size: var(--fs-0); color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: var(--fs-0); color: var(--text-2); }
  .tag { color: var(--crit); font-size: var(--fs-0); border: 1px solid var(--crit); border-radius: var(--r-1); padding: 0 4px; }
  .actions { display: flex; gap: 2px; }
  .actions button { background: none; border: 0; color: var(--text-1); cursor: pointer; padding: 2px 6px; border-radius: var(--r-1); }
  .actions button:hover { color: var(--text-0); background: var(--surface-3); }
  .rename { flex-basis: 100%; display: flex; gap: var(--sp-1); margin-top: 4px; }
  .rename input { flex: 1; background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0); border-radius: var(--r-1); padding: 4px 6px; }
  .rename button { background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0); border-radius: var(--r-1); padding: 2px 10px; cursor: pointer; }
  .dim { color: var(--text-2); font-size: var(--fs-1); }
</style>
