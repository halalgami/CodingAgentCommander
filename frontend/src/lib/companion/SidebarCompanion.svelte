<script>
  import { onMount, onDestroy } from "svelte";
  import { app } from "../stores.svelte.js";
  import { prefs } from "../prefs.svelte.js";
  import { msSinceInput, msSinceOutput } from "../termbus.js";
  import { RESOLVE_HZ } from "./pack.js";
  import { pickSlot, selectedSession } from "./slots.js";

  // Slot names are the format's vocabulary; these are what a person would say.
  // Kept here rather than in pack.js because it is presentation, and pack.js is
  // the frozen contract project B generates against.
  const SLOT_VERB = {
    idle: "waiting", working: "working", done: "finished",
    awaiting: "waiting on you", error: "hit an error", bored: "bored",
  };
  import { createMood } from "./mood.js";
  import { resolve, hydrateMemo } from "./resolver.js";
  import { buildFacts, withLatchedError, createDwell } from "./facts.js";
  import { createLimiter } from "./limiter.js";
  import { BubbleQueue } from "./bubbles.js";

  const TICK_MS = 1000 / RESOLVE_HZ;     // 500ms
  const MAX_TICK_FAILURES = 3;           // consecutive; see safeTick
  const KEYSTROKE_FREEZE_MS = 2000;      // "just after a keystroke" (§4.3)
  const OUTPUT_FREEZE_MS = 600;          // "while the pty is streaming"
  const BREATH_HZ = 0.25;                // breath AND drift share one frequency

  // Reduced motion is read ONCE at mount, not tracked. A media-query change
  // mid-session is vanishingly rare and a live listener here would be a second
  // reactive source competing with the tick.
  const reduced =
    typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  // A slot change fades through the panel BACKDROP: out, swap, in. Never an
  // A->B cross-dissolve — two generated variants are rarely the same person
  // pixel-for-pixel and a straight dissolve between two faces is the textbook
  // ghosting morph (§4.3, defect 31). Under reduced motion the fade is zero,
  // which makes it an instant swap rather than a hard cut through black.
  const FADE_MS = reduced ? 0 : 200;

  // Whole device pixels only. At dpr 2 this is 2 CSS px (4 device px); at dpr 1
  // it is 2 CSS px (2 device px). A fractional value moirés.
  const dpr = typeof devicePixelRatio === "number" && devicePixelRatio > 0 ? devicePixelRatio : 1;
  const scanlinePitch = Math.max(2, Math.round(2 * dpr)) / dpr;

  const mood = createMood({ now: () => Date.now() });
  const dwell = createDwell();
  const limiter = createLimiter({ base: 1 });
  const bubbles = new BubbleQueue();  // ttl kept live from prefs below
  let memo = hydrateMemo(null, Date.now());
  let finishTimes = [];
  let lastFinishSeq = -1;

  let shownID = $state("");        // the id currently in the <img>
  let visible = $state(1);         // 0..1, driven through the backdrop
  let lum = $state(1);             // limiter output -> filter: brightness()
  let frozen = $state(false);
  let bubble = $state(null);
  // The dwell's held slot, mirrored into $state. `dwell.held()` itself reads a
  // plain closure variable, not a rune, so a template expression that calls it
  // directly is never re-evaluated after the first render (verified: the
  // attribute froze at its initial value while other $state-backed attributes
  // on the same element kept updating tick over tick). Setting this from
  // inside tick() gives data-slot a real reactive source.
  let heldSlot = $state(null);
  let pendingID = null;
  let fadeTimer = null;
  let iv = null;

  const pack = $derived(app.companionPack);
  const hasPack = $derived(!!pack && !!pack.slots && Object.keys(pack.slots).length > 0);
  const selectedID = $derived(app.sessionKey.split(":")[0] || app.companionState.selected);
  const activeVariant = $derived(variantByID[shownID]);
  // Motion is allowed under the same rule the ambient wash uses: the user
  // opted into ambient motion, the OS is not asking for reduced motion, and
  // we are not frozen for output/keystrokes. When it is not allowed we serve
  // the still `file` poster, so the animation stops on a real frame instead of
  // an <img> looping under a paused CSS layer.
  const motionAllowed = $derived(prefs.ambientMotion && !reduced && !frozen);
  const src = $derived(
    !shownID ? ""
      : (motionAllowed && activeVariant?.motionId)
        ? "/media/pack/" + activeVariant.motionId
        : "/media/pack/" + shownID
  );

  // Cover-fit has to crop somewhere, and where it crops decides whether the
  // face is in frame. The format carries focusX/focusY per variant for exactly
  // this, and the anchor was hard-coded to 50%/0% until a generated pack
  // arrived whose subject was not centred — the model composes each image
  // independently, so one slot puts her right of centre and the next centres
  // her. Go defaults an absent focusX to 0.5 and focusY to 0, so a pack that
  // says nothing renders exactly as it did before.
  const variantByID = $derived.by(() => {
    const m = {};
    for (const list of Object.values(pack?.slots ?? {})) {
      for (const v of list ?? []) if (v?.id) m[v.id] = v;
    }
    return m;
  });
  // Read from the same snapshot the ladder resolves against, so the caption can
  // never name a different session from the one the art is showing.
  const captionName = $derived.by(() => {
    const sel = selectedSession(app.companionState);
    return sel?.name ?? "";
  });

  // The frame FILLS the band rather than keeping the art's own shape. An
  // earlier revision locked it to the pack's declared canvas aspect so
  // cover-fit had nothing to cut, which framed the figure identically at every
  // height — but it also meant the band grew while the art did not, leaving
  // dead background around a stuck image (measured: a 250x365 figure sitting
  // in a 475px band at a wide sidebar). Filling is the deliberate trade: the
  // picture is then scaled to fit that box by object-fit: contain, so it is
  // never cropped and never leaves a gutter the band does not own.

  const artPosition = $derived.by(() => {
    const v = variantByID[shownID];
    const x = Number.isFinite(v?.focusX) ? Math.min(1, Math.max(0, v.focusX)) : 0.5;
    const y = Number.isFinite(v?.focusY) ? Math.min(1, Math.max(0, v.focusY)) : 0;
    return `${(x * 100).toFixed(1)}% ${(y * 100).toFixed(1)}%`;
  });

  function showVariant(id) {
    if (!id) return;
    // Mid-fade, the resolver can revert to the variant already on screen. The
    // early return alone would leave pendingID armed, so the timer would still
    // commit the OTHER variant and a second full fade would be needed to undo
    // it. Cancel the pending swap instead and fade back in on what is shown.
    if (id === shownID) {
      if (pendingID !== null) {
        clearTimeout(fadeTimer);
        pendingID = null;
        visible = 1;
      }
      return;
    }
    if (id === pendingID) return;
    pendingID = id;
    visible = 0;
    clearTimeout(fadeTimer);
    fadeTimer = setTimeout(() => {
      shownID = pendingID;
      pendingID = null;
      visible = 1;
    }, FADE_MS);
  }

  function tick() {
    const now = Date.now();
    // Live, so dragging the slider in Settings retimes the notice already on
    // screen rather than only the next one.
    bubbles.ttlMs = Math.round(prefs.noticeSeconds * 1000);

    // Freeze while the pty is streaming and for ~2s after a keystroke. Output
    // means the user is reading; a keystroke means they are composing.
    frozen =
      msSinceOutput(selectedID, now) < OUTPUT_FREEZE_MS ||
      msSinceInput(now) < KEYSTROKE_FREEZE_MS;

    // The error latch attributes the deck's app:error to the selected session.
    const state = withLatchedError(
      { ...app.companionState, selected: selectedID },
      app.errorMs, now,
    );

    // The finish log feeds `streak`; facts.js windows it to ten minutes.
    const seq = state.finishSeq ?? 0;
    if (lastFinishSeq >= 0 && seq > lastFinishSeq) finishTimes = [...finishTimes, now];
    lastFinishSeq = seq;

    const facts = buildFacts({
      state, nowMs: now,
      msSinceInput: msSinceInput(now),
      msSinceOutput: msSinceOutput(selectedID, now),
      finishTimes,
      // firstRunOfDay is computed once in loadAll(), off the local-day-
      // rollover marker (dayRollover.js); it holds true for the whole
      // session on the first launch of a new calendar day. freshInstall
      // comes from Go (App.freshInstall via CompanionState), true for the
      // whole session on the very first run ever. Both are "event"/"ambient"
      // conditions per pack.js's CONDITION_CLASS, not per-tick pulses — the
      // resolver's own edge/level handling (conditions.js) is what turns a
      // constant-true fact into a once-only egg or a held ambient level.
      firstRunOfDay: app.firstRunOfDay === true,
      freshInstall: state.freshInstall === true,
    });
    finishTimes = facts.recentFinishMs;

    const slot = dwell.gate(pickSlot(state, facts), now);
    heldSlot = slot;
    const m = mood.update(state, facts);
    const out = resolve({ slot, pack, mood: m, facts, nowMs: now, memo });
    memo = out.memo;
    if (out.variant) showVariant(out.variant.id);

    // One scalar into one governor, applied to one composited element.
    const wash = prefs.ambientMotion && !reduced && !frozen
      ? 1 + 0.03 * Math.sin(2 * Math.PI * BREATH_HZ * (now / 1000))
      : 1;
    lum = limiter.apply(wash, now, { frozen });

    bubbles.ingest(state);
    bubble = bubbles.tick(now);
  }

  onMount(() => {
    // A tick can throw transiently — a pack reloaded into a half-written shape,
    // say. Killing the interval on the first throw froze the figure, bubble and
    // state on a stale frame for the rest of the session, silently: the user
    // sees a companion that looks fine and has quietly stopped reacting.
    // So: tolerate a few consecutive failures (skip those frames), and if it is
    // still failing, rethrow so <svelte:boundary> removes the figure honestly
    // rather than leaving a lie on screen. Any success resets the count.
    let consecutiveFailures = 0;
    const safeTick = () => {
      try {
        tick();
        consecutiveFailures = 0;
      } catch (e) {
        consecutiveFailures++;
        console.error(`companion tick failed (${consecutiveFailures}/${MAX_TICK_FAILURES})`, e);
        if (consecutiveFailures >= MAX_TICK_FAILURES) {
          clearInterval(iv);
          iv = null;
          throw e;
        }
      }
    };
    safeTick();
    iv = setInterval(safeTick, TICK_MS);
  });

  onDestroy(() => {
    clearInterval(iv);
    clearTimeout(fadeTimer);
    iv = null;
    fadeTimer = null;
  });
</script>

<div
  class="region"
  class:frozen
  class:still={!prefs.ambientMotion}
  data-testid="sidebar-companion"
  data-slot={heldSlot ?? ""}
  style:--lum={lum}
  style:--fade={FADE_MS + "ms"}
  style:--scanline-pitch={scanlinePitch + "px"}
>
  {#if hasPack}
    <figure class="frame" data-testid="companion-frame">
      {#if src}
        <img class="art" data-testid="companion-art" {src} alt="" draggable="false"
          style:opacity={visible} style:object-position={artPosition} />
      {/if}
      <div class="wash" aria-hidden="true"></div>
      <div class="vignette" aria-hidden="true"></div>
      {#if prefs.scanlines}<div class="scanlines" aria-hidden="true"></div>{/if}
      <!-- The figure reflects the SELECTED session only, which is invisible
           when several are running and all you can see is one face. Naming it
           is the difference between "something is working" and knowing what. -->
      <figcaption class="caption" data-testid="companion-caption">
        <span class="state">{SLOT_VERB[heldSlot] ?? heldSlot ?? ""}</span>
        {#if captionName}<span class="who" title={captionName}>{captionName}</span>{/if}
      </figcaption>
    </figure>
  {:else}
    <!-- No pack: no figure at all. A procedural figure needs anatomy, lighting
         and per-slot poses to look good, and a featureless silhouette reads as a
         blob at this size and sets the wrong expectation (§5.11). -->
    <div class="setup" data-testid="companion-setup">
      <p class="title">Set up your companion</p>
      <p class="hint">
        Point Commander at a pack folder in Settings. A pack is a folder with a
        <code>manifest.json</code> and portrait images.
      </p>
      <p class="hint mono">docs/companion-pack-format.md</p>
    </div>
  {/if}

  <!-- Below the figure, so it never covers the face (§4.7). -->
  {#if bubble}
    <button class="bubble" data-testid="companion-bubble"
      onclick={() => { bubbles.dismiss(Date.now()); bubble = null; }}>
      {bubble}
    </button>
  {/if}
</div>

<style>
  .region {
    flex: 1; min-height: 0; min-width: 0;
    display: flex; flex-direction: column; gap: var(--sp-2);
    /* The frame is centred in whatever room the band has, rather than stretched
       to fill it. */
    align-items: center; justify-content: flex-start;
    /* Opaque, on the app's own colour, so a pack with a transparent or dark
       matte sits on a surface rather than a hole (§4.2). */
    background: var(--surface-1);
    border-top: 1px solid var(--border-0);
    padding-top: var(--sp-2);
    /* The single output-stage governor's result, applied once, to the whole
       composited region. Nothing else in this file animates opacity or
       brightness on a schedule. */
    filter: brightness(var(--lum, 1));
  }

  .frame {
    position: relative;
    /* Takes the whole band: full width, and whatever height the region has left
       under the caption chrome. min-height: 0 is REQUIRED — the default
       min-height: auto refuses to shrink a flex item below its content, so the
       image's intrinsic height would push the frame out of the band on the way
       down. .art's object-fit: contain then scales the picture to fit it. */
    flex: 1; width: 100%; min-height: 0;
    border-radius: var(--r-2); overflow: hidden;
    background: var(--surface-1);
  }

  /* Cover-fit crops the portrait wherever the frame ends, which reads as
     "the image ran out" rather than as a figure bleeding off. A short fade to
     the panel colour makes the cut deliberate. Pointer-events off so it never
     intercepts a click meant for the art. */
  .caption {
    position: absolute; left: 0; right: 0; bottom: 0; z-index: 1;
    display: flex; align-items: baseline; gap: 6px;
    padding: 4px var(--sp-2) 6px;
    font-size: var(--fs-0); line-height: 1.3;
    /* Sits ON the art, inside the same fade the frame already draws, so it
       needs no band of its own. */
    text-shadow: 0 1px 3px oklch(0% 0 0 / 0.85);
    pointer-events: none;
  }
  .caption .state { color: var(--text-1); flex: none; }
  .caption .who {
    color: var(--text-0); font-weight: 600;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }

  .frame::after {
    content: ""; position: absolute; inset: auto 0 0 0; height: 44px;
    pointer-events: none;
    background: linear-gradient(to bottom, transparent, var(--surface-1));
  }

  .art {
    width: 100%; height: 100%; display: block;
    /* contain, not cover: the whole picture is always visible, scaled to the
       largest size that fits the band. cover filled the box instead, which for
       a wide pack meant dragging the band taller just zoomed in — at a 515px
       band a 1024x336 image was drawn 1570px wide and only 26% of it was on
       screen. Whatever a pack's aspect, the figure is now shown entire and
       scales with the box the surrounding UI leaves it (§4.1, §5.2).
       object-position still places it (and is overridden inline per variant
       from focusX/focusY); under contain it decides where any letterbox space
       falls rather than what gets cropped away. */
    object-fit: contain; object-position: 50% 0%;
    transition: opacity var(--fade, 200ms) linear;
    /* Breath and drift share ONE phase and ONE frequency. An earlier draft
       specified 0.2Hz and 0.25Hz separately, which beats every 20 seconds. */
    animation: breath 4s ease-in-out infinite;
    will-change: transform, opacity;
  }
  @keyframes breath {
    0%, 100% { transform: translateY(0) scale(1); }
    50%      { transform: translateY(-2px) scale(1.006); }
  }
  /* Suppression: pause, never reset. A reset snaps the figure, which is a
     bigger visual event than the motion it suppresses. */
  .region.frozen .art, .region.still .art { animation-play-state: paused; }

  /* Decorative only. The wash suggests the monitor as the light source. */
  .wash {
    position: absolute; inset: 0; pointer-events: none;
    background: linear-gradient(90deg, var(--accent-faint) 0%, transparent 55%);
    opacity: 0.5;
  }
  .vignette {
    position: absolute; inset: 0; pointer-events: none;
    background: radial-gradient(120% 80% at 50% 30%, transparent 45%, var(--surface-0) 100%);
    opacity: 0.7;
  }
  /* Off by default. Pitch is set from JS to a whole number of device pixels —
     a fractional pitch moirés on a Retina display — and contrast stays under 6%. */
  .scanlines {
    position: absolute; inset: 0; pointer-events: none;
    background: repeating-linear-gradient(
      to bottom, rgb(0 0 0 / 6%) 0 1px, transparent 1px var(--scanline-pitch, 2px));
  }

  .setup {
    flex: 1; min-height: 0; display: flex; flex-direction: column;
    align-items: flex-start; justify-content: center; gap: var(--sp-2);
    padding: var(--sp-3); border-radius: var(--r-2);
    border: 1px dashed var(--border-0); background: var(--surface-1);
  }
  .setup .title { margin: 0; color: var(--text-0); font-size: var(--fs-2); }
  .setup .hint { margin: 0; color: var(--text-2); font-size: var(--fs-0); line-height: 1.5; }
  .mono { font-family: var(--font-mono); word-break: break-all; }

  .bubble {
    flex: none; text-align: left; cursor: pointer;
    background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--accent-dim); border-radius: var(--r-2);
    padding: var(--sp-2); font-size: var(--fs-1); line-height: 1.4;
  }

  @media (prefers-reduced-motion: reduce) {
    .art { animation: none; transition: none; }
    .wash, .scanlines { display: none; }
  }
  /* Reduced transparency and increased contrast both mean "stop decorating". */
  @media (prefers-reduced-transparency: reduce) {
    .wash, .vignette { display: none; }
  }
  @media (prefers-contrast: more) {
    .wash, .vignette { display: none; }
  }
</style>
