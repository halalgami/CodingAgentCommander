<script>
  import Drawer from "./Drawer.svelte";
  import { app, openURL } from "../stores.svelte.js";
  import { doc, openDoc } from "../docview.svelte.js";
  import { renderProjectDoc, openProjectDoc } from "../projectdocs.js";
  import { DOC_SANDBOX, docSrcdoc, classifyDocLink, resolveDocRel, joinDocPath } from "../docs.js";

  let srcdoc = $state("");
  let links = $state([]);
  let kind = $state("");
  let error = $state("");
  let loading = $state(false);
  let copied = $state(false);
  let seq = 0; // stale-response guard: a fast link hop must not be repainted by the older render

  // Refresh is the whole staleness story: no watcher, because watching a
  // repository is a resource question a button answers for free.
  async function load() {
    const root = doc.root, rel = doc.rel;
    if (!root || !rel) return;
    const mine = ++seq;
    loading = true;
    try {
      // Go renders and sanitizes; this component parses nothing (spec R2.2).
      const r = await renderProjectDoc(root, rel);
      if (mine !== seq) return;
      srcdoc = docSrcdoc(r.html, r.css);
      kind = r.kind;
      links = (r.links ?? [])
        .map((l) => ({ ...l, kind: classifyDocLink(l.href) }))
        .filter((l) => l.kind !== "inert");
      error = "";
    } catch (e) {
      if (mine !== seq) return;
      // A refusal from the guard is the interesting failure, and readable:
      // binary and oversized files both land here, and both are exactly what
      // "Open externally" exists for.
      srcdoc = ""; links = []; error = "" + e;
    }
    if (mine === seq) loading = false;
  }

  const key = $derived(doc.root + "::" + doc.rel);
  $effect(() => { key; load(); });

  function followLink(l) {
    if (l.kind === "external") { openURL(l.href); return; }
    const rel = resolveDocRel(doc.rel, l.href);
    // Resolved here, re-validated by the Go guard; null means it left the
    // project and is not offered at all.
    if (rel) openDoc(doc.root, rel);
  }

  async function copyPath() {
    try {
      await navigator.clipboard.writeText(joinDocPath(doc.root, doc.rel));
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch { /* no clipboard permission: the path is on screen anyway */ }
  }

  async function openExternally() {
    try { await openProjectDoc(doc.root, doc.rel); } catch (e) { error = "" + e; }
  }
</script>

<Drawer title={doc.rel || "DOCUMENT"} testid="drawer-docview" wide onclose={() => (app.drawer = null)}>
  <!--
    Drawer.svelte's `.body` is a plain block (padding + overflow-y: auto,
    flex: 1 as a child of the drawer's own column flex layout) — it is not
    itself a flex container, so a flex:1 iframe dropped straight into it does
    nothing. This wrapper turns THIS component's whole body into the flex
    column instead, filling `.body`'s content box (hence height: 100%) so the
    iframe's flex: 1 has a real column to grow inside. Everything else here
    (actions, path, the collapsed link list) sizes to content as before; only
    the frame absorbs the leftover height. min-height: 0 on both the wrapper
    and the iframe is required — flex children default to a min-height equal
    to their content size, which would refuse to shrink and bring the old
    double-scrollbar right back.
  -->
  <div class="docview-body">
    <div class="actions">
      <button data-testid="docview-back" onclick={() => (app.drawer = "docs")}>← All documents</button>
      <button data-testid="docs-refresh" onclick={load} disabled={loading}>Refresh</button>
      <button data-testid="docs-external" onclick={openExternally}>Open externally</button>
      <button data-testid="docs-copy" onclick={copyPath}>{copied ? "Copied" : "Copy path"}</button>
    </div>
    <p class="path mono" data-testid="docs-path">{joinDocPath(doc.root, doc.rel)}</p>

    {#if error}
      <p class="err" data-testid="docs-error">{error}</p>
    {:else}
      <!--
        Layer one of three. The HTML inside came from a file in a repository that
        may not be ours, and this app's Go bindings live on `window` out here.
        sandbox="" grants NOTHING: no allow-scripts, so nothing executes, and no
        allow-same-origin, so the frame's origin is opaque and the parent is
        unreachable from inside. Never add either.

        Because the origin is opaque the parent cannot observe anything in here,
        including a link click — which is why Go strips every href and the
        targets are listed below instead. It is also why Escape does not close
        the drawer while the frame holds focus; the ✕ and the backdrop do.
      -->
      <iframe
        data-testid="docs-frame" title={doc.rel} sandbox={DOC_SANDBOX} {srcdoc}
      ></iframe>
    {/if}

    {#if links.length}
      <!--
        Collapsed by default (Fix 2): the list used to sit below the frame
        unconditionally, eating height the frame needed. It stays collapsed
        even for a short list — always-collapsed is one behaviour to reason
        about instead of a size-dependent exception — so specs that need it
        open (the click/follow specs) open the summary first, same as a user
        would.
      -->
      <details class="links-details">
        <summary>Links in this document ({links.length})</summary>
        <ul class="links" data-testid="docs-links">
          {#each links as l, i (l.href + ":" + i)}
            <li>
              <button data-testid="docs-link-{i}" onclick={() => followLink(l)}>
                <span class="ltext">{l.text || l.href}</span>
                <span class="lkind mono">{l.kind}</span>
              </button>
            </li>
          {/each}
        </ul>
      </details>
    {/if}
  </div>
</Drawer>

<style>
  .docview-body { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .actions { display: flex; gap: var(--sp-1); margin-bottom: var(--sp-2); flex: 0 0 auto; }
  .actions button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-1); padding: 4px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  .actions button:hover:not(:disabled) { background: var(--surface-3); }
  .path {
    color: var(--text-2); font-size: var(--fs-0); margin: 0 0 var(--sp-2);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 0 0 auto;
  }
  iframe {
    width: 100%; flex: 1; min-height: 0; border: 1px solid var(--border-0);
    border-radius: var(--r-2); background: var(--surface-0);
  }
  .links-details { flex: 0 0 auto; margin-top: var(--sp-2); }
  .links-details summary {
    cursor: pointer; font-size: var(--fs-1); color: var(--text-1);
    text-transform: uppercase; letter-spacing: 0.08em; padding: var(--sp-1) 0;
  }
  .links { list-style: none; margin: var(--sp-2) 0 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .links button {
    width: 100%; display: flex; justify-content: space-between; gap: var(--sp-2);
    background: none; border: 0; color: var(--text-0); cursor: pointer;
    padding: 4px 6px; border-radius: var(--r-1); font-size: var(--fs-1); text-align: left;
  }
  .links button:hover { background: var(--surface-2); }
  .ltext { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lkind { color: var(--text-2); font-size: var(--fs-0); }
  .mono { font-family: var(--font-mono); }
  .err { color: var(--crit); font-size: var(--fs-1); }
</style>
