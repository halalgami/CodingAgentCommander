<script>
  // The 4-word check, wherever the user is (UX spec §9). It can open while
  // someone is typing in the terminal, so it never acts on a stray key:
  // focus lands on the dialog, not a button; Enter/Space on the dialog do
  // nothing; Esc declines.
  import { rooms, act } from "./rooms.svelte.js";
  import { netLabel } from "./rooms.js";
  const p = $derived(rooms.status?.pending);
  let el = $state(null);
  let now = $state(Date.now());
  $effect(() => { if (p) { const iv = setInterval(() => (now = Date.now()), 1000); return () => clearInterval(iv); } });
  // Armed only after a moment: keys typed in the terminal as it opens (a
  // shell-completion Tab, then Enter) must never reach Allow (final review I1).
  const ARM_MS = 1500;
  const showing = $derived(!!p);
  let armed = $state(false);
  $effect(() => {
    if (!showing) { armed = false; return; }
    armed = false;
    const t = setTimeout(() => (armed = true), ARM_MS);
    return () => clearTimeout(t);
  });
  $effect(() => { if (showing && el) el.focus(); });
  const left = $derived(p?.until ? Math.max(0, p.until - Math.floor(now / 1000)) : 0);
  function key(e) {
    if (!armed && e.key !== "Escape") { e.preventDefault(); e.stopPropagation(); return; }
    if (e.target !== el) return; // a deliberately focused button keeps its keys
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); act("ConfirmCode", false); }
    else if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); }
  }
</script>

{#if p}
  <div class="scrim">
    <div class="dlg" role="alertdialog" aria-modal="true" aria-labelledby="rc-t" aria-describedby="rc-w" tabindex="-1" bind:this={el}
      data-testid="rooms-confirm" onkeydown={key}>
      <h3 id="rc-t">{p.name || "A machine"} wants to join {p.kind === "pair" ? "as your machine" : (p.room || "a room")} — over {netLabel(p.net)}</h3>
      <p>Check it shows these 4 words:</p>
      <p class="words" id="rc-w" data-testid="rooms-confirm-words">{(p.words || []).join(" · ")}</p>
      {#if left}<p class="dim">{Math.floor(left / 60)}:{String(left % 60).padStart(2, "0")} left</p>{/if}
      <div class="row">
        <button class="primary" data-testid="rooms-confirm-yes" disabled={!armed} onclick={() => act("ConfirmCode", true)}>Words match — allow</button>
        <button data-testid="rooms-confirm-no" onclick={() => act("ConfirmCode", false)}>Decline</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .scrim { position: fixed; inset: 0; z-index: 70; background: rgba(0, 0, 0, 0.55); display: flex; align-items: center; justify-content: center; }
  .dlg { width: min(460px, 92vw); background: var(--surface-2); border: 1px solid var(--accent, #3987e5); border-radius: var(--r-2);
    padding: var(--sp-3, 12px); display: flex; flex-direction: column; gap: var(--sp-2); color: var(--text-0); }
  .dlg:focus { outline: none; }
  h3 { margin: 0; font-size: var(--fs-2); }
  p { margin: 0; }
  .words { font-family: var(--font-mono); font-size: 1.4em; letter-spacing: 0.05em; }
  .dim { color: var(--text-2); font-size: var(--fs-0); }
  .row { display: flex; gap: var(--sp-2); }
</style>
