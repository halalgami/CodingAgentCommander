<script>
  import { onMount, onDestroy, untrack } from "svelte";
  import { app } from "../stores.svelte.js";
  import { buildCity, placeSites, activeCity, siteCountsOf } from "./scene.js";
  import { labelOf } from "../dashboard/label.js";
  import { SPRITES } from "./manifest.js";
  import { loadSprites, releaseSprites } from "./sprites.js";
  import { paintStatic, paintBlur, paintLive, hitItem } from "./painter.js";
  import { fit, zoomAt, pan as panCam, worldToView, wheelFactor, shouldRefit } from "./camera.js";
  import { placeSigns, towerTop, plateWidth, SIGN, CHIP } from "./signs.js";
  import { siteScale } from "./painter.js";
  import { toScreen } from "./iso.js";
  import { dayPhase, weatherOf } from "./world.js";
  import { activityText } from "../activity.js";

  let { rows, timeline, city, now, animate = true, onopen, onzoom } = $props();

  let overlay = $state(null);
  let host = $state(null), staticCv = $state(null), blurCv = $state(null), liveCv = $state(null);
  let vw = $state(800), vh = $state(500);
  let cam = $state(null);
  let movedAt = null; // when the user last moved the camera (refit rule)
  let sprites = new Map();
  let spritesReady = $state(0);
  let noCanvas = $state(false);
  let hover = $state(null);
  let raf = 0, focused = $state(true), destroyed = false;

  // Static half from CityView plus how many sites each district holds (they stand
  // on the plot); live half (sites) from rows every tick. Only projects with a
  // live session (owner, 2026-10-12). The key list (one entry per session) is a
  // string so a tick that changes nothing else never rebuilds the static city.
  const activeKeys = $derived(rows.map((r) => r.projectKey).filter(Boolean).sort().join("\n"));
  const base = $derived.by(() => {
    const keyed = activeKeys ? activeKeys.split("\n").map((projectKey) => ({ projectKey })) : [];
    return buildCity(activeCity(city || { districts: [] }, keyed), siteCountsOf(keyed));
  });
  const live = $derived(placeSites(base, rows, timeline, now));
  const scene = $derived({ districts: base.districts, items: base.items, sites: live.sites });
  const phase = $derived(dayPhase(new Date(now)));
  const weather = $derived(weatherOf({ rows, quotaPct: app.quotaPct, now }));
  // A string, so equal bounds from a new tick do not refit the camera.
  const boundsKey = $derived(JSON.stringify(live.bounds));
  const camera = $derived(cam || fit(JSON.parse(boundsKey), vw, vh));
  const view = (sx, sy) => worldToView(camera, sx, sy, vw, vh);
  // The view follows the city when districts or sites change, unless the user
  // moved the camera in the last 30 s (owner, 2026-10-14).
  $effect(() => {
    void boundsKey;
    untrack(() => { if (cam && shouldRefit(movedAt, Date.now())) cam = null; });
  });
  const moveCam = (c) => { cam = c; movedAt = Date.now(); };
  const fitView = () => { cam = null; movedAt = null; };
  const zoomBy = (f) => moveCam(zoomAt(camera, f, vw / 2, vh / 2, vw, vh));

  function size(cv) {
    const dpr = window.devicePixelRatio || 1;
    cv.width = Math.round(vw * dpr); cv.height = Math.round(vh * dpr);
    const ctx = cv.getContext("2d");
    ctx?.setTransform(dpr, 0, 0, dpr, 0, 0);
    return ctx;
  }

  // All three layers are sized together, only when the view size changes.
  let sized = $state(0);
  $effect(() => {
    void vw; void vh;
    if (noCanvas || !staticCv || !blurCv || !liveCv) return;
    if (!size(staticCv) || !size(blurCv) || !size(liveCv)) { noCanvas = true; return; }
    untrack(() => sized++);
  });

  // Static + blur: only when CityView, camera, size, sprites or day phase change.
  $effect(() => {
    void spritesReady; void phase; void sized;
    if (noCanvas || !sized) return;
    const b = base, c = camera;
    untrack(() => {
      paintStatic(staticCv.getContext("2d"), b, sprites, c, vw, vh, phase);
      paintBlur(blurCv.getContext("2d"), staticCv, vw, vh);
      drawLive(performance.now());
    });
  });

  function drawLive(t) {
    if (!liveCv || !sized) return;
    const ctx = liveCv.getContext("2d");
    if (!ctx) return;
    paintLive(ctx, scene, sprites, camera, vw, vh, t, weather, phase);
    window.__cityFrames = (window.__cityFrames || 0) + 1;
  }

  // Live loop: ≤ 30 fps, only while motion is allowed and the window is focused.
  $effect(() => {
    if (noCanvas || !animate || !focused) return;
    let last = 0;
    const loop = (t) => {
      raf = requestAnimationFrame(loop);
      if (t - last < 33) return;
      last = t;
      drawLive(t);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  });

  // Still: one redraw per data tick.
  $effect(() => {
    void now; void scene;
    if (!animate && !noCanvas) drawLive(performance.now());
  });

  onMount(() => {
    if (!document.createElement("canvas").getContext("2d")) { noCanvas = true; return; }
    const ro = new ResizeObserver(() => { vw = host.clientWidth; vh = host.clientHeight; });
    ro.observe(host);
    const onF = () => (focused = true), onB = () => (focused = false);
    window.addEventListener("focus", onF); window.addEventListener("blur", onB);
    focused = document.hasFocus();
    // Sprites decoded after leaving are released at once, never kept.
    loadSprites(SPRITES).then((m) => { if (destroyed) releaseSprites(m); else { sprites = m; spritesReady++; } });
    window.__cityHover = (ref) => (hover = findBuilding(ref.sessionID));
    // Keys also work while the dashboard itself holds focus (after a mode pick
    // or ⌘3), not only once the map was clicked; inputs, buttons and dialogs keep theirs.
    const onWinKey = (e) => {
      if (e.metaKey || e.ctrlKey || e.altKey || overlay?.contains(e.target)) return;
      const a = document.activeElement;
      if (a && a !== document.body && !a.matches?.('[data-testid="dash-home"]')) return;
      keys(e);
    };
    window.addEventListener("keydown", onWinKey);
    return () => { ro.disconnect(); window.removeEventListener("focus", onF); window.removeEventListener("blur", onB); window.removeEventListener("keydown", onWinKey); };
  });
  onDestroy(() => { destroyed = true; cancelAnimationFrame(raf); releaseSprites(sprites); delete window.__cityHover; });

  function findBuilding(id) {
    for (const { d } of scene.districts) for (const b of d.buildings) if (b.sessionID === id) return { ...b, district: d.label };
    return null;
  }

  // Pointer: drag pans, ⌘/pinch zoom, hover finds the building under the cursor.
  let drag = null;
  function down(e) { drag = { x: e.clientX, y: e.clientY }; }
  function move(e) {
    const r = host.getBoundingClientRect();
    if (drag) { moveCam(panCam(camera, e.clientX - drag.x, e.clientY - drag.y)); drag = { x: e.clientX, y: e.clientY }; return; }
    const it = hitItem(base, sprites, camera, vw, vh, e.clientX - r.left, e.clientY - r.top);
    hover = it?.kind === "building" ? findBuilding(it.ref.sessionID) : null;
  }
  function up() { drag = null; }
  // Scroll and pinch zoom at the cursor; drag pans (owner, 2026-10-14).
  function wheel(e) {
    const r = host.getBoundingClientRect();
    moveCam(zoomAt(camera, wheelFactor(e), e.clientX - r.left, e.clientY - r.top, vw, vh));
    e.preventDefault();
  }
  function keys(e) {
    const step = 40;
    const m = { ArrowLeft: [step, 0], ArrowRight: [-step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] }[e.key];
    if (m) { moveCam(panCam(camera, m[0], m[1])); e.preventDefault(); e.stopPropagation(); }
    else if (e.key === "+" || e.key === "=") zoomBy(1.25);
    else if (e.key === "-") zoomBy(0.8);
    else if (e.key === "0") fitView();
  }
  const plateAt = (pack) => { const p = toScreen(pack.x0 + pack.land / 2, pack.y0 + pack.land); return view(p.sx, p.sy); };
  // centreOf: a plate's centre relative to the city, where CodeCity grows from.
  function centreOf(el) {
    const p = el.getBoundingClientRect(), h = host.getBoundingClientRect();
    return { x: p.left + p.width / 2 - h.left, y: p.top + p.height / 2 - h.top };
  }
  const siteAt = (s) => { const p = toScreen(s.x + 0.5, s.y + 0.5); return view(p.sx, p.sy); };
  // Callouts: anchored at the tower top, placed clear of each other and of the
  // plates (signs.js); the plate box matches .plate's size below.
  const PLATE = { w: 260, h: 24 };
  const signs = $derived.by(() => {
    const anchors = scene.sites.map((s) => {
      const p = siteAt(s), top = towerTop(p.vy, camera.scale, s.floors), w = 56 * camera.scale * siteScale(s.floors);
      return { key: s.row.key, x: p.vx, y: top, body: { x: p.vx - w / 2, y: top, w, h: p.vy - top } };
    });
    const plates = scene.districts.map(({ d, pack }) => { const p = plateAt(pack), w = plateWidth(d.label, d.rank); return { x: p.vx - w / 2, y: p.vy, w, h: PLATE.h }; });
    return placeSigns(anchors, camera.scale, vw, vh, plates);
  });
</script>

<div class="city" data-testid="dash-city" bind:this={host}>
  {#if noCanvas}
    <ul class="list" data-testid="city-list">
      {#each scene.districts as { d } (d.key)}
        <li><strong>{d.label}</strong> · {d.rank} · {d.buildings.length} buildings{d.ruins ? " · ruins" : ""}</li>
      {/each}
      {#each scene.sites as s (s.row.key)}
        <li>{s.row.title} — {activityText(s.row)}</li>
      {/each}
    </ul>
  {:else}
    <canvas class="layer" data-testid="city-static" bind:this={staticCv} style="width:{vw}px;height:{vh}px"></canvas>
    <canvas class="layer blur" bind:this={blurCv} style="width:{vw}px;height:{vh}px" aria-hidden="true"></canvas>
    <canvas class="layer" data-testid="city-live" bind:this={liveCv} style="width:{vw}px;height:{vh}px"
      onpointerdown={down} onpointermove={move} onpointerup={up} onpointerleave={up} onwheel={wheel}></canvas>
  {/if}

  <!-- Everything readable or clickable lives here, above the blur (spec C4). -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="overlay" bind:this={overlay} data-testid="city-overlay" role="region" aria-label="Al Modon" tabindex="-1" onkeydown={keys} onwheel={wheel}>
    {#if city?.locked}<div class="note" data-testid="city-locked">Turn on project history to keep your city — only live sites are shown.</div>{/if}
    {#each scene.districts as { d, pack } (d.key)}
      {@const p = plateAt(pack)}
      <button class="plate" class:ruins={d.ruins} data-testid="city-plate" style="left:{p.vx}px;top:{p.vy}px"
        ondblclick={(e) => (city?.locked ? null : onzoom?.(d.key, d.label, centreOf(e.currentTarget)))}
        onkeydown={(e) => { if (e.key === "Enter" && !city?.locked) { e.preventDefault(); onzoom?.(d.key, d.label, centreOf(e.currentTarget)); } }} title="{d.hours.toFixed(1)} h · {d.buildings.length} buildings{d.plaques.length ? ' · ' + d.plaques.map((p) => p.name).join(', ') : ''}">
        <span>{d.label}</span><span class="rank">{d.rank}</span>
        {#if d.ruins}<span class="tag" data-testid="city-ruins">ruins</span>{/if}
        {#if pack.overflow || d.oldTownCount}
          <span class="tag" data-testid="city-more">{[pack.overflow ? `+${pack.overflow} more` : "", d.oldTownCount ? `old town ${d.oldTownCount}` : ""].filter(Boolean).join(" · ")}</span>
        {/if}
      </button>
    {/each}
    {#each scene.sites as s (s.row.key)}
      {@const b = signs.get(s.row.key)}
      {@const full = b?.mode === "full"}
      {@const box = full ? SIGN : CHIP}
      {#if b}
        <div class="sign" class:chip={!full} class:external={s.row.external} data-testid="city-sign" data-mode={b.mode}
          style="left:{b.x}px;top:{b.y}px;width:{box.w}px;height:{box.h}px;transform:scale({b.w / box.w})">
          {#if full}
            <span class="prow"><span class="p">{labelOf(s.row).project}</span>{#if s.row.external}<span class="ext">· external</span>{/if}</span>
            <span class="t">{labelOf(s.row).task}</span>
          {:else}
            <span class="prow"><span class="p">{labelOf(s.row).project}</span>{#if s.row.external}<span class="ext">· external</span>{/if}</span>
          {/if}
          <span class="row">
            {#if s.row.activity === "permission"}
              <span class="beacon" data-testid="city-beacon">Needs you</span>
              {#if !s.row.external}<button data-testid="city-review" onclick={(e) => onopen(s.row, e.currentTarget)}>Review →</button>{/if}
            {:else}
              <span class="a">{activityText(s.row)}{s.cranes.more ? ` · +${s.cranes.more} cranes` : ""}</span>
              {#if !s.row.external}<button class="open" onclick={(e) => onopen(s.row, e.currentTarget)}>Open</button>{/if}
            {/if}
          </span>
        </div>
      {/if}
    {/each}
    {#if hover}
      <div class="hover" data-testid="city-hover">
        <div class="t">{hover.title || "Untitled session"}</div>
        <div class="m">{hover.district} · {hover.hours.toFixed(1)} h · {hover.startedAt ? new Date(hover.startedAt).toLocaleDateString() : ""}</div>
      </div>
    {/if}
    <div class="zoom" data-testid="city-zoom" data-scale={camera.scale.toFixed(3)}>
      <button data-testid="city-zoom-out" title="Zoom out (−)" onclick={() => zoomBy(0.8)}>−</button>
      <button data-testid="city-fit" title="Fit the city (0)" onclick={fitView}>Fit</button>
      <button data-testid="city-zoom-in" title="Zoom in (+)" onclick={() => zoomBy(1.25)}>+</button>
    </div>
  </div>
</div>

<style>
  .city { position: absolute; inset: 0; overflow: hidden; background: linear-gradient(#e9eef2, #dfe6d6); }
  .layer { position: absolute; inset: 0; }
  /* Tilt-shift: the blurred copy shows only through the top and bottom bands. */
  .blur { pointer-events: none; -webkit-mask-image: linear-gradient(#000 0%, transparent 28%, transparent 72%, #000 100%); mask-image: linear-gradient(#000 0%, transparent 28%, transparent 72%, #000 100%); }
  .overlay { position: absolute; inset: 0; pointer-events: none; outline: none; }
  .overlay > * { pointer-events: auto; }
  .plate, .sign { position: absolute; white-space: nowrap; font-size: var(--fs-0); }
  .plate { transform: translate(-50%, 0); max-width: 260px; height: 24px; box-sizing: border-box; overflow: hidden; align-items: center; background: rgba(255,255,255,0.85); color: #333; border: 1px solid rgba(0,0,0,0.15); border-radius: 6px; padding: 2px 8px; display: flex; gap: 6px; cursor: zoom-in; }
  .plate.ruins { opacity: 0.75; }
  .rank { color: #7a6a40; }
  .tag { background: #f3ead2; border-radius: 4px; padding: 0 4px; }
  .sign { transform-origin: 0 0; box-sizing: border-box; overflow: hidden; background: var(--surface-1); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 6px; padding: 3px 8px; display: flex; flex-direction: column; gap: 2px; }
  .sign.chip { flex-direction: row; align-items: center; gap: 6px; padding: 0 6px; }
  .sign .row { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .sign .row .a { flex: 1; min-width: 0; }
  .sign.external { opacity: 0.7; }
  .sign .prow { display: flex; gap: 4px; min-width: 0; }
  .sign .prow .p { min-width: 0; }
  .sign .ext { flex: none; font-weight: 400; color: var(--text-2); }
  .sign .p { font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
  .sign .t { overflow: hidden; text-overflow: ellipsis; }
  .sign .a { color: var(--text-1); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; }
  .beacon { color: var(--warn); font-weight: 600; animation: beacon 1.6s ease-in-out infinite; }
  .sign button { align-self: flex-start; background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 4px; font-size: var(--fs-0); cursor: pointer; }
  .hover { position: absolute; left: 16px; bottom: 16px; background: var(--surface-1); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 6px; padding: 6px 10px; max-width: 360px; }
  .hover .m { color: var(--text-2); font-size: var(--fs-0); }
  .zoom { position: absolute; right: 16px; bottom: 16px; display: flex; gap: 4px; }
  .zoom button { background: var(--surface-1); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 6px; padding: 3px 10px; cursor: pointer; min-width: 28px; }
  .note { position: absolute; left: 50%; top: 12px; transform: translateX(-50%); background: var(--surface-1); color: var(--text-1); border-radius: 6px; padding: 4px 10px; font-size: var(--fs-0); }
  .list { position: absolute; inset: 0; overflow: auto; padding: var(--sp-4); margin: 0; color: var(--text-0); background: var(--surface-0); }
  @keyframes beacon { 50% { opacity: 0.45; } }
</style>
