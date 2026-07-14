<script module>
  // Module-level trigger so SettingsDrawer's "Replay intro" works without prop drilling.
  let show = $state(false);
  export function replayIntro() { show = true; }
  export function bootOnFirstRun() {
    const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const skipped = new URLSearchParams(location.search).has("nointro");
    let played = true;
    try { played = localStorage.getItem("commander.introPlayed.v1") === "1"; } catch {}
    if (!reduced && !skipped && !played) show = true;
  }
</script>

<script>
  import gsap from "gsap";

  let overlay = $state(null);

  function finish() {
    try { localStorage.setItem("commander.introPlayed.v1", "1"); } catch {}
    show = false;
  }

  $effect(() => {
    if (!show || !overlay) return;
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) { finish(); return; }
    const tris = overlay.querySelectorAll(".tri");
    tris.forEach((t) => {
      const len = t.getTotalLength();
      t.style.strokeDasharray = len;
      t.style.strokeDashoffset = len;
    });
    const tl = gsap.timeline({ onComplete: finish });
    tl.to(tris, { strokeDashoffset: 0, duration: 0.5, stagger: 0.15, ease: "power3.out" })
      .fromTo(".boot-letter", { opacity: 0, y: 8 }, { opacity: 1, y: 0, stagger: 0.035, duration: 0.3, ease: "power2.out" }, "-=0.15")
      .to(overlay, { opacity: 0, duration: 0.25, delay: 0.35 });
    const skip = () => { tl.progress(1); };
    window.addEventListener("keydown", skip, { once: true, capture: true });
    window.addEventListener("pointerdown", skip, { once: true, capture: true });
    return () => {
      tl.kill();
      window.removeEventListener("keydown", skip, true);
      window.removeEventListener("pointerdown", skip, true);
    };
  });
</script>

{#if show}
  <div class="boot" bind:this={overlay} data-testid="boot-intro">
    <svg viewBox="0 0 64 64" width="88" height="88" aria-hidden="true">
      <path class="tri" d="M32 8 L56 52 H8 Z" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linejoin="round" />
      <path class="tri" d="M32 24 L45 48 H19 Z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" opacity="0.5" />
    </svg>
    <div class="word" aria-label="COMMANDER">
      {#each "COMMANDER" as ch, i (i)}<span class="boot-letter">{ch}</span>{/each}
    </div>
  </div>
{/if}

<style>
  .boot {
    position: fixed; inset: 0; z-index: var(--layer-boot);
    background: var(--surface-0); color: var(--accent);
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    gap: var(--sp-4);
  }
  .word { display: flex; gap: 0.3em; }
  .boot-letter {
    font-size: var(--fs-4); font-weight: 600; letter-spacing: 0.1em;
    color: var(--text-0); opacity: 0;
  }
</style>
