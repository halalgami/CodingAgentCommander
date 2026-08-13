<script>
  import { app, closeLitellmInstaller, runLitellmInstall } from "../stores.svelte.js";

  const s = $derived(app.litellmInstall);

  let logEl = $state(null);
  // Keep the live log pinned to the newest line as pip streams output.
  $effect(() => {
    if (s?.log?.length && logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  function onkeydown(e) {
    // Don't let Esc close mid-install (the goroutine keeps running); allow it
    // only when idle/finished so a stray keypress can't orphan a build.
    if (e.key === "Escape" && !s.running) { e.stopPropagation(); closeLitellmInstaller(); }
  }
</script>

{#if s}
  <div class="backdrop" onclick={() => !s.running && closeLitellmInstaller()} aria-hidden="true"></div>
  <div class="modal" data-testid="litellm-install" role="dialog" aria-modal="true"
       aria-label="Install LiteLLM runtime" {onkeydown}>
    <h2>LiteLLM runtime needed</h2>
    <p class="lede">
      Routed models (OpenCode Zen, AWS Bedrock) run through a local LiteLLM proxy —
      a Python component that isn't bundled. Install it once here; Anthropic models
      never need it.
    </p>

    {#if s.done}
      <p class="ok" data-testid="litellm-install-done">✓ Installed. Relaunch your routed model.</p>
    {:else if !s.canInstall}
      <p class="warn" data-testid="litellm-install-nopython">
        No Python 3.10–3.13 found to build the runtime. The macOS system Python
        (3.9) is too old for litellm, and 3.14+ isn't supported yet. Install one
        in range — <code>brew install python@3.12</code> or python.org — then reopen this.
      </p>
      <p class="alt">Or install litellm yourself against a 3.10–3.13 Python and relaunch:</p>
      <pre class="cmd">python3.12 -m pip install --user 'litellm[proxy]==1.83.9' 'fastapi==0.124.4'</pre>
    {:else if s.error}
      <p class="warn" data-testid="litellm-install-error">Install failed: {s.error}</p>
    {:else if !s.running}
      <p class="alt">Python: <code>{s.python}</code></p>
      <p class="alt">Builds a self-contained venv (~1–2 min, needs network once).</p>
    {/if}

    {#if s.log.length}
      <pre class="log" bind:this={logEl} data-testid="litellm-install-log">{s.log.join("\n")}</pre>
    {/if}

    <div class="actions">
      {#if s.running}
        <span class="spin" data-testid="litellm-install-running">Installing…</span>
      {:else}
        <button class="ghost" data-testid="litellm-install-close" onclick={closeLitellmInstaller}>
          {s.done ? "Done" : "Cancel"}
        </button>
        {#if s.canInstall && !s.done}
          <button class="go" data-testid="litellm-install-go" onclick={runLitellmInstall}>
            {s.error ? "Retry" : "Install"}
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
    width: min(600px, 92vw); z-index: var(--layer-palette);
    background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-3);
    padding: var(--sp-4); box-shadow: 0 16px 48px oklch(0% 0 0 / 0.5);
    display: flex; flex-direction: column; gap: var(--sp-2);
    animation: pop var(--t-med) var(--ease-out);
  }
  @keyframes pop { from { transform: translateX(-50%) translateY(-6px); opacity: 0; } }
  h2 { margin: 0; font-size: var(--fs-2); font-weight: 600; letter-spacing: 0.04em; }
  .lede { margin: 0; font-size: var(--fs-1); color: var(--text-1); line-height: 1.5; }
  .alt { margin: 0; font-size: var(--fs-1); color: var(--text-2); }
  .ok { margin: 0; font-size: var(--fs-1); color: var(--ok, var(--accent)); font-weight: 600; }
  .warn { margin: 0; font-size: var(--fs-1); color: var(--crit); line-height: 1.5; }
  code { font-family: var(--font-mono); background: var(--surface-2); padding: 1px 5px; border-radius: var(--r-1); }
  .cmd {
    font-family: var(--font-mono); font-size: var(--fs-1); color: var(--text-0); margin: 0;
    background: var(--surface-2); border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 8px 10px; user-select: all; overflow-x: auto;
  }
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
