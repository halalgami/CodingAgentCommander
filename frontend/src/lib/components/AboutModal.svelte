<script>
  import { onMount } from "svelte";
  import { app, buildInfo, openURL } from "../stores.svelte.js";
  import icon from "../../assets/images/appicon.png";

  const SITE = "https://algamthe.dev";
  const CREDITS = ["Wails", "Svelte", "xterm.js", "LiteLLM", "Claude / Anthropic", "Geist Sans", "JetBrains Mono"];

  let info = $state({ version: "dev", commit: "", buildDate: "" });
  let closeBtn = $state(null);

  onMount(async () => {
    info = await buildInfo();
    closeBtn?.focus();
  });

  function onkeydown(e) {
    if (e.key === "Escape") { e.stopPropagation(); app.about = false; }
  }
  const stamp = $derived(
    "v" + info.version +
    (info.commit ? " · " + info.commit : "") +
    (info.buildDate ? " · built " + info.buildDate : "")
  );
</script>

{#if app.about}
  <div class="backdrop" onclick={() => (app.about = false)} aria-hidden="true"></div>
  <div class="modal" data-testid="about-modal" role="dialog" aria-modal="true"
       aria-label="About Commander" {onkeydown} tabindex="-1">
    <img class="icon" src={icon} alt="" width="72" height="72" />
    <h2 class="name">COMMANDER</h2>
    <p class="stamp mono" data-testid="about-version">{stamp}</p>
    <p class="tagline">The terminal is the workspace; Commander is the tower around it.</p>
    <p class="legal">© 2026 Algam · MIT License</p>
    <button class="link" data-testid="about-link" onclick={() => openURL(SITE)}>algamthe.dev</button>
    <div class="credits">
      <span class="lbl">Built with</span>
      <p>{CREDITS.join(" · ")}</p>
    </div>
    <button class="close" data-testid="about-close" bind:this={closeBtn}
            onclick={() => (app.about = false)}>Close</button>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: oklch(0% 0 0 / 0.45); z-index: var(--layer-backdrop); }
  .modal {
    position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);
    width: min(420px, 90vw); z-index: var(--layer-palette);
    background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-3);
    padding: var(--sp-4); box-shadow: 0 16px 48px oklch(0% 0 0 / 0.5);
    display: flex; flex-direction: column; align-items: center; gap: var(--sp-2);
    text-align: center; outline: none;
    animation: pop var(--t-med) var(--ease-out);
  }
  @keyframes pop { from { transform: translate(-50%, -50%) translateY(-6px); opacity: 0; } }
  .icon { border-radius: var(--r-3); }
  .name { margin: 0; font-size: var(--fs-2); font-weight: 600; letter-spacing: 0.28em; color: var(--text-0); }
  .stamp { margin: 0; font-size: var(--fs-0); color: var(--text-2); }
  .mono { font-family: var(--font-mono); }
  .tagline { margin: var(--sp-1) 0 0; font-size: var(--fs-1); color: var(--text-1); max-width: 34ch; }
  .legal { margin: 0; font-size: var(--fs-0); color: var(--text-2); }
  .link {
    background: none; border: 0; color: var(--accent); cursor: pointer;
    font-size: var(--fs-1); text-decoration: underline; padding: 2px;
  }
  .link:hover { color: var(--accent-hover); }
  .credits { margin-top: var(--sp-2); border-top: 1px solid var(--border-0); padding-top: var(--sp-2); width: 100%; }
  .credits .lbl { font-size: var(--fs-0); color: var(--text-2); text-transform: uppercase; letter-spacing: 0.08em; }
  .credits p { margin: 4px 0 0; font-size: var(--fs-0); color: var(--text-1); }
  .close {
    margin-top: var(--sp-3); background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2);
    padding: 6px 18px; cursor: pointer; font-size: var(--fs-1);
  }
  .close:hover { background: var(--surface-3); }
</style>
