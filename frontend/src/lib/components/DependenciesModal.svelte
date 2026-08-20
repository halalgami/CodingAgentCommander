<script>
  // Preflight panel for the external tools Commander shells out to. It exists so
  // a missing dependency is named here, with the command that fixes it, instead
  // of surfacing later as `exec: "tmux": executable file not found in %PATH%` on
  // a failed launch.
  import { app, closeDependencies, runPwshInstall } from "../stores.svelte.js";

  const s = $derived(app.depsModal);
  const tools = $derived(app.deps ?? []);
  const missing = $derived(tools.filter((t) => t.required && !t.found));
  // pwsh is the only tool Commander can fetch itself; tmux is bundled in the
  // release and claude is a self-updating npm package we must not freeze.
  const installable = $derived(missing.find((t) => t.canInstall));

  let logEl = $state(null);
  $effect(() => {
    if (s?.log?.length && logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  function onkeydown(e) {
    // Esc stays blocked mid-download: the goroutine keeps running, and a stray
    // keypress should not hide a 106 MB transfer that is still in flight.
    if (e.key === "Escape" && !s.running) { e.stopPropagation(); closeDependencies(); }
  }
</script>

{#if s}
  <div class="backdrop" onclick={() => !s.running && closeDependencies()} aria-hidden="true"></div>
  <div class="modal" data-testid="deps-preflight" role="dialog" aria-modal="true"
       aria-label="External dependencies" {onkeydown}>
    <h2>{missing.length ? "Missing dependencies" : "Dependencies"}</h2>
    <p class="lede">
      Commander hosts sessions in tmux windows and drives the <code>claude</code> CLI.
      These are the tools it shells out to.
    </p>

    <ul class="tools">
      {#each tools as t (t.name)}
        <li class:bad={t.required && !t.found} data-testid={`dep-${t.name}`}>
          <span class="mark">{t.found ? "✓" : "✗"}</span>
          <span class="label">{t.label}</span>
          {#if t.found}
            <span class="detail">
              {t.version || t.path}{#if t.managed}<span class="tag">bundled</span>{/if}
            </span>
          {:else}
            <code class="detail">{t.hint}</code>
          {/if}
        </li>
      {/each}
    </ul>

    {#if s.done}
      <p class="ok" data-testid="deps-install-done">✓ Installed.</p>
    {:else if s.error}
      <p class="warn" data-testid="deps-install-error">Install failed: {s.error}</p>
    {:else if installable && !s.running}
      <p class="alt">
        Commander can fetch {installable.label} for you — about 106 MB, downloaded
        once into your user profile and verified against a pinned checksum.
      </p>
    {:else if !missing.length}
      <p class="ok" data-testid="deps-all-ok">✓ Everything Commander needs is present.</p>
    {/if}

    {#if s.log.length}
      <pre class="log" bind:this={logEl} data-testid="deps-install-log">{s.log.join("\n")}</pre>
    {/if}

    <div class="actions">
      {#if s.running}
        <span class="spin" data-testid="deps-install-running">Downloading…</span>
      {:else}
        <button class="ghost" data-testid="deps-close" onclick={closeDependencies}>
          {missing.length ? "Continue anyway" : "Close"}
        </button>
        {#if installable}
          <button class="go" data-testid="deps-install-go" onclick={runPwshInstall}>
            {s.error ? "Retry" : `Install ${installable.label}`}
          </button>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: oklch(0% 0 0 / 0.45); z-index: var(--layer-backdrop); }
  .modal {
    position: fixed; top: 18%; left: 50%; transform: translateX(-50%);
    width: min(640px, 92vw); z-index: var(--layer-palette);
    background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-3);
    padding: var(--sp-4); box-shadow: 0 16px 48px oklch(0% 0 0 / 0.5);
    display: flex; flex-direction: column; gap: var(--sp-2);
    animation: pop var(--t-med) var(--ease-out);
  }
  @keyframes pop { from { transform: translateX(-50%) translateY(-6px); opacity: 0; } }
  h2 { margin: 0; font-size: var(--fs-2); font-weight: 600; letter-spacing: 0.04em; }
  .lede { margin: 0; font-size: var(--fs-1); color: var(--text-1); line-height: 1.5; }
  .alt { margin: 0; font-size: var(--fs-1); color: var(--text-2); line-height: 1.5; }
  .ok { margin: 0; font-size: var(--fs-1); color: var(--ok, var(--accent)); font-weight: 600; }
  .warn { margin: 0; font-size: var(--fs-1); color: var(--crit); line-height: 1.5; }
  .tools { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .tools li {
    display: grid; grid-template-columns: 1.2em minmax(9em, auto) 1fr; align-items: baseline;
    gap: var(--sp-2); font-size: var(--fs-1);
    background: var(--surface-2); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 10px;
  }
  .tools li.bad { border-color: var(--crit); }
  .mark { color: var(--ok, var(--accent)); font-weight: 700; }
  .tools li.bad .mark { color: var(--crit); }
  .label { color: var(--text-0); }
  .detail {
    color: var(--text-2); font-family: var(--font-mono); font-size: var(--fs-0);
    overflow-wrap: anywhere;
  }
  .tag {
    margin-left: 8px; padding: 1px 6px; border-radius: var(--r-1);
    background: var(--surface-3); color: var(--text-1); font-family: var(--font-sans, inherit);
    letter-spacing: 0.06em; font-size: var(--fs-0);
  }
  code { font-family: var(--font-mono); background: var(--surface-2); padding: 1px 5px; border-radius: var(--r-1); }
  .log {
    font-family: var(--font-mono); font-size: var(--fs-0); color: var(--text-2); margin: 0;
    background: var(--surface-0, var(--surface-2)); border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 8px 10px; max-height: 240px; overflow: auto; white-space: pre-wrap; word-break: break-all;
  }
  .actions { display: flex; justify-content: flex-end; align-items: center; gap: var(--sp-2); margin-top: var(--sp-2); }
  .spin { font-size: var(--fs-1); color: var(--text-2); letter-spacing: 0.06em; }
  .ghost {
    background: var(--surface-2); color: var(--text-1); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 14px; cursor: pointer; font-size: var(--fs-1);
  }
  .ghost:hover { color: var(--text-0); background: var(--surface-3); }
  .go {
    background: var(--accent); color: var(--on-accent); border: 0;
    border-radius: var(--r-2); padding: 6px 18px; cursor: pointer;
    font-weight: 600; letter-spacing: 0.06em; font-size: var(--fs-1);
  }
  .go:hover { background: var(--accent-hover); }
</style>
