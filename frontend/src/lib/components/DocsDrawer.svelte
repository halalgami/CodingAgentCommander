<script>
  import Drawer from "./Drawer.svelte";
  import { app } from "../stores.svelte.js";
  import { docsCtx, openDoc } from "../docview.svelte.js";
  import { listProjectDocs, listSessionDocs } from "../projectdocs.js";
  import { fuzzyFilter } from "../fuzzy.js";
  import { relDocTime, joinDocPath } from "../docs.js";

  let listing = $state({ entries: [], truncated: false, root: "" });
  let query = $state("");
  let error = $state("");
  let loading = $state(false);
  let sinceOnly = $state(false);
  let now = $state(0);

  async function load() {
    loading = true;
    try {
      // A session listing carries the sinceStart flags; a project listing
      // cannot, so the filter is only offered when there is a session.
      listing = docsCtx.windowID
        ? await listSessionDocs(docsCtx.windowID)
        : await listProjectDocs(docsCtx.root);
      now = Math.floor(Date.now() / 1000);
      error = "";
    } catch (e) {
      listing = { entries: [], truncated: false, root: "" };
      error = "" + e;
    }
    loading = false;
  }

  $effect(() => {
    const key = docsCtx.root + "::" + docsCtx.windowID;
    sinceOnly = docsCtx.sinceOnly;
    void key;
    load();
  });

  // The root Go itself validated wins over whatever (possibly stale or
  // empty) root the caller had on hand -- a session card whose stats have
  // not polled a cwd yet passes docsCtx.root = "", but the LIST still works
  // because Go resolves the session's cwd from the registry. Using Go's
  // answer for both display and every openDoc(...) call means the frontend
  // and the read/open guard can no longer disagree about the folder.
  const root = $derived(listing.root || docsCtx.root);

  const sinceStartEntries = $derived(listing.entries.filter((e) => e.sinceStart));
  // Defect: a freshly launched session has nothing at or after its launch
  // time, so the since-set is EMPTY and the filtered list was blank with
  // only a small "N older hidden" count to explain it. When that happens,
  // fall back to showing everything rather than an empty drawer -- with a
  // visible note, because a filter that silently shows nothing is the same
  // lie as a silent truncation cap. Requires at least one entry to exist at
  // all, so a genuinely empty project still gets the ordinary empty state.
  const fallbackToAll = $derived(sinceOnly && listing.entries.length > 0 && sinceStartEntries.length === 0);
  const scoped = $derived(sinceOnly && !fallbackToAll ? sinceStartEntries : listing.entries);
  const hidden = $derived(listing.entries.length - scoped.length);
  const shown = $derived(fuzzyFilter(query, scoped, (e) => e.rel).slice(0, 200));
</script>

<Drawer title="DOCUMENTS" testid="drawer-docs" onclose={() => (app.drawer = null)}>
  <input
    class="search" data-testid="docs-search" placeholder="Search documents…"
    bind:value={query}
  />
  <div class="bar">
    <button data-testid="docs-list-refresh" onclick={load} disabled={loading}>Refresh</button>
    {#if docsCtx.windowID}
      <!-- The count is part of the contract: a filter that hides things
           silently is the same lie as a silent truncation cap. The title
           documents a real gap: for a session recovered from tmux after an
           app restart, "this session" is measured from when the app first
           saw it, not from when Claude actually started. -->
      <button
        data-testid="docs-filter"
        title="Changed since this session started. For a session recovered after an app restart, that means since the app restarted, not since Claude began."
        onclick={() => (sinceOnly = !sinceOnly)}
      >
        {fallbackToAll
          ? "Showing all · nothing new this session"
          : sinceOnly ? `Showing this session · ${hidden} older hidden` : "Showing all"}
      </button>
    {/if}
  </div>
  {#if fallbackToAll}
    <p class="dim" data-testid="docs-filter-fallback">
      Nothing changed since this session started — showing all {listing.entries.length}.
    </p>
  {/if}
  <p class="root mono" data-testid="docs-root">{root}</p>

  {#if error}
    <p class="err" data-testid="docs-list-error">{error}</p>
  {:else if !shown.length}
    <p class="dim" data-testid="docs-empty">
      {query ? "No documents match." : "No documents in this project yet."}
    </p>
  {/if}

  <ul class="list" data-testid="docs-list">
    {#each shown as e (e.rel)}
      <li>
        <button data-testid="docs-row-{e.rel}" onclick={() => openDoc(root, e.rel)}>
          <span class="rel">{e.rel}{#if e.sinceStart} <span class="pip" title="changed since this session started">•</span>{/if}</span>
          <span class="meta">{relDocTime(e.modTime, now)} · {Math.max(1, Math.round(e.size / 1024))} KB</span>
        </button>
      </li>
    {/each}
  </ul>

  {#if listing.truncated}
    <p class="dim" data-testid="docs-list-truncated">
      This project has more documents than the listing cap — narrow the project or open them externally.
    </p>
  {/if}
</Drawer>

<style>
  .search {
    width: 100%; background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 6px 8px; font-size: var(--fs-1); margin-bottom: var(--sp-2);
  }
  .search:focus { outline: none; border-color: var(--accent-dim); }
  .bar { display: flex; gap: var(--sp-1); margin-bottom: var(--sp-1); }
  .bar button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-1); padding: 3px 8px; cursor: pointer; font-size: var(--fs-0);
  }
  .root {
    color: var(--text-2); font-size: var(--fs-0); margin: 0 0 var(--sp-2);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .list button {
    width: 100%; display: flex; flex-direction: column; gap: 2px; text-align: left;
    background: none; border: 0; color: var(--text-0); cursor: pointer;
    padding: 4px 6px; border-radius: var(--r-1);
  }
  .list button:hover { background: var(--surface-2); }
  .rel { font-size: var(--fs-1); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: var(--fs-0); color: var(--text-2); }
  .pip { color: var(--accent); margin-left: var(--sp-1); }
  .mono { font-family: var(--font-mono); }
  .dim { color: var(--text-2); font-size: var(--fs-1); }
  .err { color: var(--crit); font-size: var(--fs-1); }
</style>
